package webconfig

import (
	"strings"
	"testing"
)

func TestEnvRoundTrip(t *testing.T) {
	prev := "# old header\nRIDDLE_OPENAI_KEY=sk-abc\nCUSTOM_EXTRA=kept\n"
	vals := ParseEnv(prev)
	if vals["RIDDLE_OPENAI_KEY"] != "sk-abc" || vals["CUSTOM_EXTRA"] != "kept" {
		t.Fatalf("parse: %v", vals)
	}
	vals["RIDDLE_OPENAI_MODEL"] = "gemini-3.5-flash"
	out := BuildEnv(Riddle(), vals, prev)
	for _, want := range []string{"RIDDLE_OPENAI_KEY=sk-abc", "RIDDLE_OPENAI_MODEL=gemini-3.5-flash", "CUSTOM_EXTRA=kept"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "RIDDLE_OPENAI_MAX_TOKENS=") {
		t.Errorf("empty field should be omitted:\n%s", out)
	}
}
