package wallet

import (
	"context"
	"errors"
	"flag"
	"io"
)

func passwordCommand(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("password", flag.ContinueOnError)
	fs.SetOutput(errout)
	var f commandFlags
	f.add(fs)
	nextFile := fs.String("new-password-file", "", "private new passphrase file; otherwise BL_WALLET_NEW_PASSWORD")
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
	if err := ctx.Err(); err != nil {
		return err
	}
	old, err := secret(f.passwordFile, "BL_WALLET_PASSWORD")
	if err != nil {
		return err
	}
	defer clear(old)
	next, err := secret(*nextFile, "BL_WALLET_NEW_PASSWORD")
	if err != nil {
		return err
	}
	defer clear(next)
	if err = ChangePassword(f.path, old, next); err != nil {
		return err
	}
	return output(out, map[string]string{"operation": "password", "status": "complete"})
}
