package device

import (
	"os"
	"strings"
	"testing"
)

// Live smoke test: go test ./internal/device -run KeyOnly with REMAGIC_TEST_HOST set.
func TestConnectKeyOnlyLive(t *testing.T) {
	host := os.Getenv("REMAGIC_TEST_HOST")
	if host == "" {
		t.Skip("REMAGIC_TEST_HOST not set")
	}
	d, err := ConnectKeyOnly(host)
	if err != nil {
		t.Fatalf("ConnectKeyOnly: %v", err)
	}
	defer d.Close()
	out, err := d.Run("echo key-only-ok")
	if err != nil || !strings.Contains(out, "key-only-ok") {
		t.Fatalf("run: %v %q", err, out)
	}
}
