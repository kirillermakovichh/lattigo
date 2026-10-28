package wallet

import (
	"context"
	"errors"
	"flag"
	"io"
)

func outboxCommand(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("outbox", flag.ContinueOnError)
	fs.SetOutput(errout)
	var f commandFlags
	f.add(fs)
	action := fs.String("action", "inspect", "inspect stored signed intent")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := f.resolveAccountName(fs); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if *action != "inspect" {
		return errors.New("outbox action must be inspect")
	}
	entry, err := InspectOutbox(f.path, f.index)
	if err != nil {
		return err
	}
	return output(out, entry)
}
