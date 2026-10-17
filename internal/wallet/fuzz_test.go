package wallet

import "testing"

func FuzzEncryptedEnvelope(f *testing.F) {
	for _, seed := range []string{`{}`, `null`, `{"version":1,"kdf":"pbkdf2-sha256","iterations":600000}`, `{"iterations":-1}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxFileSize {
			t.Skip()
		}
		seed, err := decrypt(data, []byte("fuzz-test-password"))
		if err == nil && len(seed) != 32 {
			t.Fatal("wallet accepted an invalid seed length")
		}
		clear(seed)
	})
}
