package wallet

import "sort"

type NamedAccount struct {
	Name  string `json:"name"`
	Index uint64 `json:"index"`
}

func AccountNames(path string) ([]NamedAccount, error) {
	base, err := canonicalWallet(path)
	if err != nil {
		return nil, err
	}
	p, err := readProfile(base)
	if err != nil {
		return nil, err
	}
	rows := make([]NamedAccount, 0, len(p.Accounts))
	for name, index := range p.Accounts {
		rows = append(rows, NamedAccount{Name: name, Index: index})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows, nil
}
func NameAccount(path, name string, index uint64, replace bool) error {
	return editProfile(path, func(p *profile) error {
		if _, exists := p.Accounts[name]; exists && !replace {
			return ErrNameExists
		}
		p.Accounts[name] = index
		return nil
	})
}
func ForgetAccountName(path, name string) error {
	return editProfile(path, func(p *profile) error {
		if _, exists := p.Accounts[name]; !exists {
			return ErrNameNotFound
		}
		delete(p.Accounts, name)
		return nil
	})
}
func AccountIndex(path, name string) (uint64, error) {
	base, err := canonicalWallet(path)
	if err != nil {
		return 0, err
	}
	p, err := readProfile(base)
	if err != nil {
		return 0, err
	}
	index, exists := p.Accounts[name]
	if !exists {
		return 0, ErrNameNotFound
	}
	return index, nil
}
