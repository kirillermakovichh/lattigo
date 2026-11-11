package rpc

import (
	"bytes"
	"errors"
	"net/http"
	"runtime/pprof"
	"strconv"
	"time"
)

const maxProfileBytes = 16 << 20

type profileBuffer struct {
	bytes.Buffer
	failed bool
}

func (b *profileBuffer) Write(p []byte) (int, error) {
	if len(p) > maxProfileBytes-b.Len() {
		b.failed = true
		return 0, errors.New("profile exceeds 16 MiB")
	}
	return b.Buffer.Write(p)
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	if !s.authenticated(w, r) {
		return
	}
	q := r.URL.Query()
	if len(r.URL.RawQuery) > 128 {
		writeError(w, 400, "invalid_profile", "profile query too long")
		return
	}
	for k, v := range q {
		if (k != "kind" && k != "seconds") || len(v) != 1 {
			writeError(w, 400, "invalid_profile", "unknown or repeated field")
			return
		}
	}
	kind := q.Get("kind")
	if kind != "heap" && kind != "goroutine" && kind != "cpu" {
		writeError(w, 400, "invalid_profile", "kind must be heap, goroutine or cpu")
		return
	}
	seconds := 1
	if v := q.Get("seconds"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5 || kind != "cpu" {
			writeError(w, 400, "invalid_profile", "seconds requires cpu and must be 1..5")
			return
		}
		seconds = n
	}
	select {
	case s.profileSlots <- struct{}{}:
		defer func() { <-s.profileSlots }()
	default:
		writeError(w, 503, "overloaded", "profile collection already active")
		return
	}
	var b profileBuffer
	var err error
	if kind == "cpu" {
		if err = pprof.StartCPUProfile(&b); err == nil {
			timer := time.NewTimer(time.Duration(seconds) * time.Second)
			select {
			case <-timer.C:
			case <-r.Context().Done():
			case <-s.stopping:
			}
			timer.Stop()
			pprof.StopCPUProfile()
		}
	} else {
		err = pprof.Lookup(kind).WriteTo(&b, 0)
	}
	if r.Context().Err() != nil {
		return
	}
	if err != nil || b.failed || b.Len() == 0 {
		writeError(w, 503, "profile_unavailable", "cannot collect bounded profile")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+kind+`.pprof"`)
	w.WriteHeader(200)
	_, _ = w.Write(b.Bytes())
}
