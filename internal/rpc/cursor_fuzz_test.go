package rpc

import (
	"encoding/base64"
	"strings"
	"testing"
)

func FuzzCursor(f *testing.F) {
	codec := cursorCodec{42}
	token, err := codec.encode(cursorFixture())
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{token, "", token + "=", "%", strings.Repeat("x", maxCursorBytes+1)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		cursor, err := codec.decode(raw)
		if err != nil {
			return
		}
		if len(raw) > maxCursorBytes || cursor.validate() != nil {
			t.Fatal("invalid cursor accepted")
		}
		canonical, err := codec.encode(cursor)
		if err != nil {
			t.Fatal(err)
		}
		again, err := codec.decode(canonical)
		if err != nil || again != cursor {
			t.Fatal("cursor changed on round trip")
		}
		if _, err := (cursorCodec{43}).decode(raw); err == nil {
			t.Fatal("foreign signing key accepted cursor")
		}
		bytes, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			t.Fatal(err)
		}
		bytes[len(bytes)-1] ^= 1
		if _, err := codec.decode(base64.RawURLEncoding.EncodeToString(bytes)); err == nil {
			t.Fatal("tampered cursor authenticated")
		}
	})
}
