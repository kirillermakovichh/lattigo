package wallet

import (
	"context"
	"errors"
	"flag"
	"io"
)

func receiveBatchCommand(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("receive-all", flag.ContinueOnError)
	fs.SetOutput(errout)
	var f commandFlags
	f.add(fs)
	limit := fs.Int("limit", 20, "maximum confirmed receives (1..100)")
	representative := fs.String("representative", "", "representative only when opening an account")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if f.timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if err := f.resolveAccountName(fs); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	result, err := receiveBatch(ctx, f, *limit, *representative)
	if err != nil {
		result.Error = err.Error()
	}
	if outputErr := output(out, result); outputErr != nil {
		return errors.Join(err, outputErr)
	}
	return err
}
