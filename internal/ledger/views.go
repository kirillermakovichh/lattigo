package ledger

import (
	"errors"
	"sort"

	"example.com/go-block-lattice/internal/protocol"
)

var ErrStaleView = errors.New("page snapshot is unavailable or changed")

type Anchor struct {
	Height uint64        `json:"height"`
	Tip    protocol.Hash `json:"tip"`
}
type HistoryView struct {
	Anchor Anchor
	Items  []Record
	Before uint64
	More   bool
}
type ReceivableView struct {
	Anchor Anchor
	Items  []Receivable
	After  protocol.Hash
	More   bool
}

// anchorLocked requires s.mu. Historical anchors survive appended records;
// receivable views require the current anchor because consumption changes membership.
func (s *Store) anchorLocked(anchor Anchor, current bool) (Anchor, error) {
	height := uint64(len(s.state.records))
	if anchor.Tip == "" {
		if anchor.Height != 0 {
			return Anchor{}, ErrInvalid
		}
		return Anchor{Height: height, Tip: s.state.tip}, nil
	}
	if anchor.Height > height {
		return Anchor{}, ErrStaleView
	}
	tip := s.state.network
	if anchor.Height > 0 {
		tip = s.state.records[anchor.Height-1].ID()
	}
	if anchor.Tip != tip || (current && (anchor.Height != height || anchor.Tip != s.state.tip)) {
		return Anchor{}, ErrStaleView
	}
	return anchor, nil
}
func (s *Store) HistoryView(a protocol.Account, anchor Anchor, before uint64, limit int) (HistoryView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := HistoryView{Items: []Record{}}
	if s.closed {
		return result, ErrClosed
	}
	if _, err := a.PublicKey(); err != nil {
		return result, ErrInvalid
	}
	if limit < 1 || limit > 1000 {
		return result, ErrInvalid
	}
	initial := anchor.Tip == ""
	view, err := s.anchorLocked(anchor, false)
	if err != nil {
		return result, err
	}
	result.Anchor = view
	if initial {
		before = view.Height
	}
	if before > view.Height {
		return result, ErrInvalid
	}
	retained := uint64(2)
	for i := int(before); i > 0; i-- {
		r := s.state.records[i-1]
		if r.Block.Account != a {
			continue
		}
		size := uint64(s.state.recordSizes[i-1]) + 1
		if len(result.Items) >= limit || (len(result.Items) > 0 && retained+size > 16<<20) {
			result.More = true
			result.Before = result.Items[len(result.Items)-1].Height - 1
			break
		}
		result.Items = append(result.Items, copyRecord(r))
		retained += size
	}
	return result, nil
}
func (s *Store) ReceivableView(a protocol.Account, anchor Anchor, after protocol.Hash, limit int) (ReceivableView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := ReceivableView{Items: []Receivable{}}
	if s.closed {
		return result, ErrClosed
	}
	if _, err := a.PublicKey(); err != nil {
		return result, ErrInvalid
	}
	if limit < 1 || limit > 1000 {
		return result, ErrInvalid
	}
	if after != "" {
		if _, err := after.Bytes(); err != nil {
			return result, ErrInvalid
		}
	}
	view, err := s.anchorLocked(anchor, true)
	if err != nil {
		return result, err
	}
	result.Anchor = view
	for _, row := range s.state.receivables {
		if row.To == a && row.Source > after {
			result.Items = append(result.Items, row)
		}
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].Source < result.Items[j].Source })
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.More = true
		result.After = result.Items[len(result.Items)-1].Source
	}
	return result, nil
}
