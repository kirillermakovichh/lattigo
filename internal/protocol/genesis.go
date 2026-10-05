package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
)

type Allocation struct {
	Account        Account `json:"account"`
	Balance        Amount  `json:"balance"`
	Representative Account `json:"representative"`
}
type Genesis struct {
	Version     uint8        `json:"version"`
	Name        string       `json:"name"`
	Supply      Amount       `json:"supply"`
	WorkBits    uint8        `json:"work_bits"`
	Allocations []Allocation `json:"allocations"`
}

func (g Genesis) Validate() error {
	if g.Version != 1 {
		return errors.New("unsupported genesis version")
	}
	if len(g.Name) < 1 || len(g.Name) > 128 || strings.TrimSpace(g.Name) != g.Name {
		return errors.New("invalid network name")
	}
	if g.WorkBits > 24 {
		return errors.New("work bits exceed 24")
	}
	if g.Supply == 0 {
		return errors.New("supply must be positive")
	}
	if len(g.Allocations) == 0 || len(g.Allocations) > 4096 {
		return errors.New("genesis needs 1..4096 allocations")
	}
	seen := make(map[Account]bool)
	var sum uint64
	for _, a := range g.Allocations {
		if _, e := a.Account.PublicKey(); e != nil {
			return fmt.Errorf("allocation: %w", e)
		}
		if _, e := a.Representative.PublicKey(); e != nil {
			return fmt.Errorf("representative: %w", e)
		}
		if seen[a.Account] {
			return errors.New("duplicate allocation account")
		}
		seen[a.Account] = true
		if a.Balance == 0 || math.MaxUint64-sum < uint64(a.Balance) {
			return errors.New("invalid or overflowing allocation")
		}
		sum += uint64(a.Balance)
	}
	if sum != uint64(g.Supply) {
		return errors.New("allocations do not match supply")
	}
	return nil
}
func (g Genesis) ID() Hash {
	if g.Validate() != nil {
		return ""
	}
	p := []byte("BL/genesis/v1")
	p = append(p, g.Version)
	p = binary.BigEndian.AppendUint16(p, uint16(len(g.Name)))
	p = append(p, []byte(g.Name)...)
	p = binary.BigEndian.AppendUint64(p, uint64(g.Supply))
	p = append(p, g.WorkBits)
	a := append([]Allocation(nil), g.Allocations...)
	sort.Slice(a, func(i, j int) bool { return a[i].Account < a[j].Account })
	p = binary.BigEndian.AppendUint32(p, uint32(len(a)))
	for _, v := range a {
		pub, _ := v.Account.PublicKey()
		rep, _ := v.Representative.PublicKey()
		p = append(p, pub...)
		p = binary.BigEndian.AppendUint64(p, uint64(v.Balance))
		p = append(p, rep...)
	}
	return Digest(p)
}
func LoadGenesis(path string) (Genesis, error) {
	var g Genesis
	f, e := os.Open(path)
	if e != nil {
		return g, e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if e = d.Decode(&g); e != nil {
		return g, e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return g, errors.New("trailing genesis content")
	}
	return g, g.Validate()
}
