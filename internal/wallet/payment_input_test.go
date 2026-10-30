package wallet

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/payment"
)

func TestPaymentFileInputIsBoundedWithoutSecretPermissions(t *testing.T) {
	_, c := profileFixture(t)
	r := payment.Request{Address: c.Address, Network: c.Network, Amount: 1}
	raw, err := payment.EncodeFile(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "invoice.json")
	if err = os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	got, provided, err := paymentInput("", path)
	if err != nil || !provided || got != r {
		t.Fatal("public request file changed payment")
	}
	uri, err := r.URI()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := paymentInput(uri, path); err == nil {
		t.Fatal("ambiguous request source accepted")
	}
	if _, provided, err := paymentInput("", ""); err != nil || provided {
		t.Fatal("absent request fabricated payment")
	}
}
func TestPaymentFileRejectsFIFOWithoutWaitingForWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readPaymentFile(path); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO request accepted")
		}
	case <-time.After(time.Second):
		writer, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		writer.Close()
		<-done
		t.Fatal("request input waited for FIFO writer")
	}
}
