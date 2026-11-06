package main

import (
	"strings"
	"testing"
)

// A whitespace-only path is no path at all: `deja blame "   "` and
// `deja blame -- "   "` sailed past the missing-path check because the check
// only looked for an empty string, and a leading empty argument was silently
// ignored instead of refused (#5938).
func TestBlameWhitespaceOnlyPathIsMissing(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"whitespace only", []string{"   "}},
		{"empty", []string{""}},
		{"empty before a path", []string{"", "main.go"}},
		{"whitespace before a path", []string{"   ", "main.go"}},
		{"whitespace after a path", []string{"main.go", "   "}},
		{"after terminator whitespace only", []string{"--", "   "}},
		{"after terminator empty", []string{"--", ""}},
		{"after terminator empty before a path", []string{"--", "", "main.go"}},
		{"after terminator whitespace before a path", []string{"--", "   ", "main.go"}},
		{"after terminator whitespace after a path", []string{"--", "main.go", "   "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := parseBlame(tc.args)
			if err == nil {
				t.Fatalf("parseBlame(%v) returned no error", tc.args)
			}
			if !strings.Contains(err.Error(), "blame needs a path") {
				t.Errorf("parseBlame(%v) = %v, want the missing-path refusal", tc.args, err)
			}
		})
	}

	// Real paths still parse as expected.
	path, _, _, err := parseBlame([]string{"main.go"})
	if err != nil {
		t.Fatalf("parseBlame([main.go]) error: %v", err)
	}
	if path != "main.go" {
		t.Errorf("parsed path = %q, want %q", path, "main.go")
	}

	pathDash, _, _, err := parseBlame([]string{"--", "main.go"})
	if err != nil {
		t.Fatalf("parseBlame([-- main.go]) error: %v", err)
	}
	if pathDash != "main.go" {
		t.Errorf("parsed path = %q, want %q", pathDash, "main.go")
	}

	pathLine, _, _, err := parseBlame([]string{"main.go:42"})
	if err != nil {
		t.Fatalf("parseBlame([main.go:42]) error: %v", err)
	}
	if pathLine != "main.go:42" {
		t.Errorf("parsed path = %q, want %q", pathLine, "main.go:42")
	}

	pathJSON, _, mode, err := parseBlame([]string{"--json", "main.go"})
	if err != nil {
		t.Fatalf("parseBlame([--json main.go]) error: %v", err)
	}
	if pathJSON != "main.go" || !mode.JSON {
		t.Errorf("parseBlame([--json main.go]) path=%q json=%v, want main.go and true", pathJSON, mode.JSON)
	}
}
