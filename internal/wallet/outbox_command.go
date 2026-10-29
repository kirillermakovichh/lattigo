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
	action := fs.String("action", "inspect", "inspect or retry stored signed intent")
	wait := fs.Bool("wait", false, "wait for confirmation when retrying")
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
	if f.timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	if *action == "retry" {
		return retryOutbox(ctx, f, *wait, out)
	}
	if *action != "inspect" {
		return errors.New("outbox action must be inspect or retry")
	}
	entry, err := InspectOutbox(f.path, f.index)
	if err != nil {
		return err
	}
	return output(out, entry)
}
