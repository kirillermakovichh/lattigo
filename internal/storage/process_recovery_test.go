package storage

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

// The child is the actual test executable in a separate process. The parent
// kills it without running deferred Close after an acknowledged durable boundary.
func TestJournalInterruptionChild(t *testing.T) {
	directory := os.Getenv("BL_STORAGE_INTERRUPTION_DIRECTORY")
	if directory == "" {
		return
	}
	stage := os.Getenv("BL_STORAGE_INTERRUPTION_STAGE")
	journal, err := Open(directory, []byte("process-recovery-v1"), func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.SaveSigning([]byte(`{"height":2,"intent":"durable"}`)); err != nil {
		t.Fatal(err)
	}
	if stage == "after-append" {
		if err := journal.Append([]byte("second")); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("durable-boundary")
	// Keep the descriptor and lock live until the parent terminates the process.
	if _, err := os.Stdin.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	t.Fatal("interruption child unexpectedly resumed")
}

func TestProcessKillPreservesJournalAndSigningBoundaries(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"before-append", "after-append"} {
		t.Run(stage, func(t *testing.T) {
			directory := t.TempDir()
			journal, err := Open(directory, []byte("process-recovery-v1"), func([]byte) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if err := journal.Append([]byte("first")); err != nil {
				t.Fatal(err)
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, executable, "-test.run=^TestJournalInterruptionChild$", "-test.timeout=12s")
			child.Env = append(os.Environ(), "BL_STORAGE_INTERRUPTION_DIRECTORY="+directory, "BL_STORAGE_INTERRUPTION_STAGE="+stage)
			input, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			child.Stderr = &stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if child.ProcessState == nil {
					child.Process.Kill()
					child.Wait()
				}
			}()
			scanner := bufio.NewScanner(output)
			if !scanner.Scan() || scanner.Text() != "durable-boundary" {
				child.Process.Kill()
				child.Wait()
				t.Fatalf("child missed durable boundary: %v %s", scanner.Err(), stderr.String())
			}
			if err := child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := child.Wait(); err == nil {
				t.Fatal("child exited without interruption")
			}
			var replayed []string
			recovered, err := OpenStrict(directory, []byte("process-recovery-v1"), func(raw []byte) error { replayed = append(replayed, string(raw)); return nil })
			if err != nil {
				t.Fatal(err)
			}
			defer recovered.Close()
			want := []string{"first"}
			if stage == "after-append" {
				want = append(want, "second")
			}
			if !reflect.DeepEqual(replayed, want) {
				t.Fatalf("recovery changed durable records: %v", replayed)
			}
			signing, err := recovered.Signing()
			if err != nil || string(signing) != `{"height":2,"intent":"durable"}` {
				t.Fatalf("durable signing intent disappeared: %s %v", signing, err)
			}
			// The killed process must have released its exclusive filesystem lock.
			if err := recovered.SaveSigning(signing); err != nil {
				t.Fatal(err)
			}
		})
	}
}
