package wallet

import (
	"errors"
	"flag"
	"io"
)

func namesCommand(args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("names", flag.ContinueOnError)
	fs.SetOutput(errout)
	var f commandFlags
	f.add(fs)
	action := fs.String("action", "list", "list, add or remove")
	name := fs.String("name", "", "local account label")
	replace := fs.Bool("replace", false, "explicitly replace an existing label")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch *action {
	case "list":
		rows, err := AccountNames(f.path)
		if err != nil {
			return err
		}
		return output(out, rows)
	case "add":
		if err := NameAccount(f.path, *name, f.index, *replace); err != nil {
			return err
		}
	case "remove":
		if err := ForgetAccountName(f.path, *name); err != nil {
			return err
		}
	default:
		return errors.New("account name action must be list, add or remove")
	}
	return output(out, map[string]string{"name": *name, "status": "complete"})
}
