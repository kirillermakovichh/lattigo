//go:build faultaudit

package node

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigurationAndRepresentativeIOFailures(t *testing.T) {
	for _, point := range _auditPoints {
		if point.Function != "LoadConfig" && point.Function != "loadRepresentative" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input")
			raw := []byte(strings.Repeat("0", 64))
			if point.Function == "LoadConfig" {
				cfg := Config{Genesis: "genesis.json"}
				cfg.defaults()
				var err error
				raw, err = json.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			var err error
			if point.Function == "LoadConfig" {
				_, err = LoadConfig(path)
			} else {
				_, err = loadRepresentative(path)
			}
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if !errors.Is(err, _auditInjected) {
				t.Fatalf("input I/O error lost: %v", err)
			}
		})
	}
}
