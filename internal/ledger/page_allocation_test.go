package ledger

import "testing"

func TestPagesAllocateForRequestedResults(t *testing.T) {
	s, a, b, _ := viewFixture(t)
	if got := s.History(a, 0, 1); len(got) != 1 || cap(got) > 1 {
		t.Fatal("history allocation exceeds requested count")
	}
	if got := s.Records(0, 1); len(got) != 1 || cap(got) > 1 {
		t.Fatal("bootstrap allocation exceeds requested count")
	}
	h, err := s.HistoryView(a, Anchor{}, 0, 1)
	if err != nil || len(h.Items) != 1 || cap(h.Items) > 1 {
		t.Fatal(h, err)
	}
	r, err := s.ReceivableView(b, Anchor{}, "", 1)
	if err != nil || len(r.Items) != 1 || cap(r.Items) > 1 {
		t.Fatal(r, err)
	}
}
