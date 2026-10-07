package p2p

import "testing"

func TestPeerURLsRestrictDialTargets(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:7071", "https://node.example:443/", "http://[::1]:7071"} {
		if _, err := ValidatePeer(raw); err != nil {
			t.Errorf("valid peer %q: %v", raw, err)
		}
	}
	for _, raw := range []string{"file:///tmp/key", "http://user:password@localhost:7071", "http://localhost:7071/secret", "http://localhost:7071?q=1", "http://localhost:7071#fragment", "http://localhost:99999", "localhost:7071"} {
		if _, err := ValidatePeer(raw); err == nil {
			t.Errorf("accepted unsafe peer %q", raw)
		}
	}
}
