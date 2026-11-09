package p2p

import "net/http"

// Capabilities is additive discovery; the protocol1 status schema stays strict.
// It advertises implemented bounds rather than negotiating weaker verification.
type Capabilities struct {
	Version          uint8    `json:"version"`
	Transports       []string `json:"transports"`
	MaxMessageBytes  int      `json:"max_message_bytes"`
	MaxResponseBytes int      `json:"max_response_bytes"`
	MaxRecords       int      `json:"max_records"`
	PriorityBurst    int      `json:"priority_burst"`
}

func (n *Network) capabilities(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, Capabilities{Version: 1, Transports: []string{"http-json"}, MaxMessageBytes: maxMessage, MaxResponseBytes: maxResponse, MaxRecords: 16, PriorityBurst: 4})
}
