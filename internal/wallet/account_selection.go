package wallet

import (
	"errors"
	"flag"
)

func (f *commandFlags) resolveAccountName(fs *flag.FlagSet) error {
	if f.accountName == "" {
		return nil
	}
	explicit := false
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == "index" {
			explicit = true
		}
	})
	if explicit {
		return errors.New("--account-name and --index cannot be combined")
	}
	index, err := AccountIndex(f.path, f.accountName)
	if err != nil {
		return err
	}
	f.index = index
	return nil
}
