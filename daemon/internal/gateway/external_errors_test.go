package gateway

import (
	"net/http"
	"testing"
)

func TestMapDaemonErrorToExternal_PairCodeInvalid(t *testing.T) {
	status, code, message := mapDaemonErrorToExternal("E_PAIR_CODE_INVALID")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", status, http.StatusBadRequest)
	}
	if code != "E_PAIR_CODE_INVALID" {
		t.Fatalf("code=%q, want %q", code, "E_PAIR_CODE_INVALID")
	}
	if message == "" {
		t.Fatal("message should not be empty")
	}
}
