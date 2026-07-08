package main

import (
	_ "embed"
	"strconv"
	"strings"
)

// Upstream v0.5.3 hooks target OS <=3.27. On 3.28+ the sidebar QML moved and
// appload panics with "Couldn't resolve the hashed identifier … AppLoad hooks".
// Built from asivery/rm-appload PR #59 until a stable upstream release ships.
//
//go:embed embedded/appload-3.28-aarch64.so
var appload328 []byte

func osNeedsAppLoad328(osv string) bool {
	parts := strings.Split(strings.TrimSpace(osv), ".")
	if len(parts) < 2 || parts[0] != "3" {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	return err == nil && minor >= 28
}

func apploadSOForOS(osv string) ([]byte, string) {
	if osNeedsAppLoad328(osv) {
		return appload328, "3.28 (PR #59 build — upstream v0.5.3 breaks on this OS)"
	}
	return nil, ""
}
