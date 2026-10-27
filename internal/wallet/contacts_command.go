package wallet

import (
	"errors"
	"flag"
	"io"

	"example.com/go-block-lattice/internal/protocol"
)

func contactsCommand(args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("contacts", flag.ContinueOnError)
	fs.SetOutput(errout)
	var f commandFlags
	f.add(fs)
	action := fs.String("action", "list", "list, add or remove")
	name := fs.String("name", "", "contact name")
	address := fs.String("address", "", "recipient account")
	network := fs.String("network", "", "recipient network hash")
	replace := fs.Bool("replace", false, "explicitly replace an existing contact")
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
		rows, err := Contacts(f.path)
		if err != nil {
			return err
		}
		return output(out, rows)
	case "add":
		if err := PutContact(f.path, *name, Contact{Address: protocol.Account(*address), Network: protocol.Hash(*network)}, *replace); err != nil {
			return err
		}
	case "remove":
		if err := DeleteContact(f.path, *name); err != nil {
			return err
		}
	default:
		return errors.New("contact action must be list, add or remove")
	}
	return output(out, map[string]string{"name": *name, "status": "complete"})
}
