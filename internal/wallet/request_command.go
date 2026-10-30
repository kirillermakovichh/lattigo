package wallet

import (
	"context"
	"errors"
	"flag"
	"io"

	"example.com/go-block-lattice/internal/payment"
	"example.com/go-block-lattice/internal/protocol"
)

type requestResult struct {
	URI string `json:"uri"`
	payment.Request
}

func requestOutput(f commandFlags, out io.Writer, r payment.Request) error {
	uri, err := r.URI()
	if err != nil {
		return err
	}
	if f.out != "" {
		if err := WritePrivate(f.out, []byte(uri+"\n")); err != nil {
			return err
		}
	}
	return output(out, requestResult{URI: uri, Request: r})
}
func requestCommand(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("request", flag.ContinueOnError)
	fs.SetOutput(errout)
	var f commandFlags
	f.add(fs)
	address := fs.String("address", "", "recipient address; defaults to selected wallet account")
	network := fs.String("network", "", "explicit genesis hash")
	amount := fs.String("amount", "", "positive raw amount")
	label := fs.String("label", "", "optional recipient label")
	message := fs.String("message", "", "optional request message")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := f.resolveAccountName(fs); err != nil {
		return err
	}
	value, err := protocol.ParseAmount(*amount)
	if err != nil {
		return err
	}
	recipient := protocol.Account(*address)
	if recipient == "" {
		w, err := f.open()
		if err != nil {
			return err
		}
		recipient, err = keyAccount(w, f.index)
		w.Close()
		if err != nil {
			return err
		}
	}
	return requestOutput(f, out, payment.Request{Address: recipient, Network: protocol.Hash(*network), Amount: value, Label: *label, Message: *message})
}
