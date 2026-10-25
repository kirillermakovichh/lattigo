//go:build faultaudit

package rpc

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackupStagingIOFailures(t *testing.T) {
	for _, point := range _auditPoints {
		if point.Function != "backup" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			s, _ := branchServer(t)
			req := httptest.NewRequest("GET", "/v1/admin/backup", nil)
			req.Header.Set("Authorization", "Bearer private-branch-token")
			out := httptest.NewRecorder()
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			s.Handler().ServeHTTP(out, req)
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if out.Code != 503 || strings.Contains(out.Body.String(), _auditInjected.Error()) {
				t.Fatalf("backup staging error response: %d %s", out.Code, out.Body.String())
			}
			if len(s.backupSlot) != 0 {
				t.Fatal("failed staging retained backup admission")
			}
		})
	}
}
