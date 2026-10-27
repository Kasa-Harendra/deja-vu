package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// `deja sources` answers "where did deja look", and the empty-state advice
// points people at it. Its harness list was hand-maintained and had fallen four
// behind the registry: cline, roo, pi and openclaw were absent while their
// sessions — 33 of them here — sat in the index.
func TestSourcesListsEveryHarnessInTheRegistry(t *testing.T) {
	hermeticEnv(t)
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = b.ReadFrom(r)
		done <- b.String()
	}()
	printSources(t.TempDir())
	_ = w.Close()
	os.Stdout = old
	out := <-done

	for _, h := range sources.Registry() {
		if !strings.Contains(out, h.Name) {
			t.Errorf("deja sources never mentions %q, so nobody can tell whether deja looked there", h.Name)
		}
	}

	// The security model tells a reader that an exclude line has to name one of
	// the stores this command prints, and it spells that count in words. It said
	// twenty-five while the command printed thirty-five — ten stores a reader
	// would not know they could exclude. The number is the row count, not the
	// registry's: deja prints its own store too.
	rows := 0
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) != "" {
			rows++
		}
	}
	word, ok := countWords[rows]
	if !ok {
		t.Fatalf("deja sources prints %d rows and this test has no word for it; add one", rows)
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "SECURITY-MODEL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "the " + word + " `deja sources` prints"; !strings.Contains(string(b), want) {
		t.Errorf("docs/SECURITY-MODEL.md does not say %q — deja sources prints %d rows", want, rows)
	}
}

// Every row names where deja looks, even before the harness is installed, so
// someone who installs it later knows where its history should appear. Four
// rows built their location only from directories that exist and printed an
// empty column on a fresh machine (#5638).
func TestSourcesNamesALocationForEveryStoreOnAnEmptyHome(t *testing.T) {
	hermeticEnv(t)
	var out bytes.Buffer
	printSourcesTo(&out, t.TempDir())

	rows := 0
	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rows++
		fields := strings.Split(line, "\t")
		if len(fields) < 2 || strings.TrimSpace(fields[1]) == "" {
			t.Errorf("deja sources prints no location for %q on an empty home: %q", fields[0], line)
		}
	}
	if rows == 0 {
		t.Fatal("deja sources printed no rows")
	}
}

// The four rows that fall back name the default only while deja has found
// nothing: on an empty home each names its own default, and once a store
// exists the row names the roots deja reads, joined as before (#5638).
func TestSourcesFallbackRowsNameTheDefaultUntilAStoreExists(t *testing.T) {
	hermeticEnv(t)
	for _, env := range []string{"DEJA_COPILOT_CHAT_ROOTS", "DEJA_ROO_ROOTS", "DEJA_KILO_ROOTS", "DEJA_CHERRYSTUDIO_ROOTS"} {
		t.Setenv(env, "")
	}
	t.Setenv("DEJA_ROO_CLI_ROOT", filepath.Join(t.TempDir(), "absent"))
	locations := func() map[string]string {
		var out bytes.Buffer
		printSourcesTo(&out, t.TempDir())
		got := map[string]string{}
		for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
			if fields := strings.Split(line, "\t"); len(fields) >= 2 {
				got[fields[0]] = fields[1]
			}
		}
		return got
	}
	check := func(stage string, want map[string]string) {
		t.Helper()
		got := locations()
		for name, w := range want {
			if got[name] != w {
				t.Errorf("%s: %s location = %q, want %q", stage, name, got[name], w)
			}
		}
	}

	check("empty home", map[string]string{
		"copilot-chat": sources.CopilotChatDefaultRoot(),
		"roo":          sources.RooDefaultRoot(),
		"kilocode":     sources.KiloDefaultRoot(),
		"cherrystudio": sources.CherryStudioDefaultRoot(),
	})

	// Stores in hosts other than the default, so a row that kept naming the
	// default would not match.
	base := filepath.Dir(filepath.Dir(sources.CopilotChatDefaultRoot()))
	codium := filepath.Join(base, "VSCodium", "User")
	cursor := filepath.Join(base, "Cursor", "User")
	rooCodium := filepath.Join(codium, "globalStorage", sources.RooExtensionID)
	rooCursor := filepath.Join(cursor, "globalStorage", sources.RooExtensionID)
	kilo := filepath.Join(codium, "globalStorage", sources.KiloExtensionID)
	cherryPi := filepath.Join(filepath.Dir(sources.CherryStudioDefaultRoot()), ".pi", "sessions")
	for _, d := range []string{rooCodium, rooCursor, kilo, cherryPi} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sep := string(os.PathListSeparator)
	check("stores present", map[string]string{
		"copilot-chat": codium,
		"roo":          rooCodium + sep + rooCursor,
		"kilocode":     kilo,
		"cherrystudio": cherryPi,
	})
}
