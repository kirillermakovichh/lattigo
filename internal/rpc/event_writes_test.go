package rpc

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

type eventWriter struct {
	header                             http.Header
	deadline                           time.Time
	writes, flushes                    int
	failWrite, failFlush, failDeadline bool
}

func (w *eventWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *eventWriter) WriteHeader(int) {}
func (w *eventWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.failWrite {
		return 0, errors.New("write")
	}
	return len(p), nil
}
func (w *eventWriter) FlushError() error {
	w.flushes++
	if w.failFlush {
		return errors.New("flush")
	}
	return nil
}
func (w *eventWriter) SetWriteDeadline(deadline time.Time) error {
	if !deadline.IsZero() {
		w.deadline = deadline
	}
	if w.failDeadline {
		return errors.New("deadline")
	}
	return nil
}
func TestEventWritesHaveIndependentDeadlines(t *testing.T) {
	for _, tc := range []struct {
		name    string
		writer  eventWriter
		large   bool
		wantErr bool
	}{
		{name: "bounded"}, {name: "write", writer: eventWriter{failWrite: true}, wantErr: true},
		{name: "flush", writer: eventWriter{failFlush: true}, wantErr: true}, {name: "deadline", writer: eventWriter{failDeadline: true}, wantErr: true},
		{name: "oversized", large: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(`{}`)
			if tc.large {
				data = make([]byte, maxEventData+1)
			}
			start := time.Now()
			err := writeEvent(&tc.writer, http.NewResponseController(&tc.writer), "id", data)
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
			if !tc.large && (tc.writer.deadline.Before(start) || tc.writer.deadline.After(time.Now().Add(eventWriteTimeout))) {
				t.Fatal("unbounded write deadline")
			}
			if tc.large && tc.writer.writes != 0 {
				t.Fatal("oversized frame published")
			}
		})
	}
}

type idleDeadlineWriter struct {
	eventWriter
	active time.Time
}

func (w *idleDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.active = deadline
	return w.eventWriter.SetWriteDeadline(deadline)
}
func TestEventFrameClearsIdleStreamDeadline(t *testing.T) {
	writer := &idleDeadlineWriter{}
	if err := writeEvent(writer, http.NewResponseController(writer), "position", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if !writer.active.IsZero() {
		t.Fatal("idle HTTP/2 stream retains write deadline")
	}
	if writer.deadline.IsZero() {
		t.Fatal("frame had no write deadline")
	}
}
