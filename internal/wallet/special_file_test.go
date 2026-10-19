package wallet

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPrivateInputRejectsFIFOWithoutWaitingForWriter(t *testing.T) {
	for _, kind := range []string{"secret", "block"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret.fifo")
			if err := syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				var err error
				if kind == "secret" {
					_, err = ReadSecret(path)
				} else {
					_, err = loadBlock(path)
				}
				done <- err
			}()
			<-started
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("FIFO accepted as a private file")
				}
			case <-time.After(time.Second):
				writer, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
				if err != nil {
					t.Fatal(err)
				}
				writer.Close()
				<-done
				t.Fatal("private input blocked waiting for a FIFO writer")
			}

		})
	}
}
