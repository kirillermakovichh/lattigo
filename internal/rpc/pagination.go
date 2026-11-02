package rpc

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

type pageScope struct {
	Account protocol.Account
	Anchor  ledger.Anchor
	Before  uint64
	After   protocol.Hash
	Limit   int
}
type pageEnvelope[T any] struct {
	Items  []T           `json:"items"`
	Anchor ledger.Anchor `json:"anchor"`
	Next   string        `json:"next,omitempty"`
}

func (s *Server) pageScope(r *http.Request, kind string) (pageScope, error) {
	var scope pageScope
	if len(r.URL.RawQuery) > 4096 {
		return scope, errCursor
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return scope, errCursor
	}
	for key, values := range query {
		if (key != "address" && key != "limit" && key != "cursor") || len(values) != 1 {
			return scope, errCursor
		}
	}
	scope.Account = protocol.Account(query.Get("address"))
	if _, err := scope.Account.PublicKey(); err != nil {
		return scope, errCursor
	}
	scope.Limit = 100
	if value := query.Get("limit"); value != "" {
		scope.Limit, err = strconv.Atoi(value)
		if err != nil || scope.Limit < 1 || scope.Limit > 1000 {
			return scope, errCursor
		}
	}
	if token := query.Get("cursor"); token != "" {
		p, err := s.cursors.decode(token)
		if err != nil {
			return scope, err
		}
		if p.Account != scope.Account || p.Kind != kind || p.Network != s.cfg.Store.Status().Network {
			return scope, errCursor
		}
		if query.Get("limit") != "" && scope.Limit != p.Limit {
			return scope, errCursor
		}
		scope.Anchor = ledger.Anchor{Height: p.Height, Tip: p.Tip}
		scope.Before = p.Before
		scope.After = p.After
		scope.Limit = p.Limit
	}
	return scope, nil
}
func (s *Server) nextCursor(kind string, scope pageScope, anchor ledger.Anchor, before uint64, after protocol.Hash) (string, error) {
	return s.cursors.encode(pageCursor{Version: 1, Network: s.cfg.Store.Status().Network, Kind: kind, Account: scope.Account, Height: anchor.Height, Tip: anchor.Tip, Before: before, After: after, Limit: scope.Limit})
}
func pageError(w http.ResponseWriter, err error) {
	if errors.Is(err, ledger.ErrStaleView) {
		writeError(w, 409, "stale_cursor", "page snapshot changed; restart pagination")
		return
	}
	classify(w, err)
}
func (s *Server) historyPage(w http.ResponseWriter, r *http.Request) {
	scope, err := s.pageScope(r, "history")
	if err != nil {
		writeError(w, 400, "invalid_cursor", errCursor.Error())
		return
	}
	view, err := s.cfg.Store.HistoryView(scope.Account, scope.Anchor, scope.Before, scope.Limit)
	if err != nil {
		pageError(w, err)
		return
	}
	response := pageEnvelope[ledger.Record]{Items: view.Items, Anchor: view.Anchor}
	if view.More {
		response.Next, err = s.nextCursor("history", scope, view.Anchor, view.Before, "")
		if err != nil {
			pageError(w, err)
			return
		}
	}
	writeJSON(w, 200, response)
}

func (s *Server) receivablePage(w http.ResponseWriter, r *http.Request) {
	scope, err := s.pageScope(r, "receivables")
	if err != nil {
		writeError(w, 400, "invalid_cursor", errCursor.Error())
		return
	}
	view, err := s.cfg.Store.ReceivableView(scope.Account, scope.Anchor, scope.After, scope.Limit)
	if err != nil {
		pageError(w, err)
		return
	}
	response := pageEnvelope[ledger.Receivable]{Items: view.Items, Anchor: view.Anchor}
	if view.More {
		response.Next, err = s.nextCursor("receivables", scope, view.Anchor, 0, view.After)
		if err != nil {
			pageError(w, err)
			return
		}
	}
	writeJSON(w, 200, response)
}
