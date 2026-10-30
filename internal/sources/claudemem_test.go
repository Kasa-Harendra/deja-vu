package sources

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
)

const testClaudeMemSchema = `
create table sdk_sessions (
  sdk_session_id text primary key,
  claude_session_id text,
  project text,
  created_at_epoch integer,
  completed_at_epoch integer,
  last_activity_epoch integer
);

create table user_prompts (
  id integer primary key autoincrement,
  sdk_session_id text,
  prompt_text text,
  created_at_epoch integer
);

create table observations (
  id integer primary key autoincrement,
  sdk_session_id text,
  title text,
  subtitle text,
  narrative text,
  text text,
  facts text,
  files_modified text,
  created_at_epoch integer
);

create table session_summaries (
  id integer primary key autoincrement,
  sdk_session_id text,
  request text,
  investigated text,
  learned text,
  completed text,
  next_steps text,
  notes text,
  created_at_epoch integer
);
`

func createTestClaudeMemDB(t *testing.T, sql string) string {
	t.Helper()
	if !SQLite3Available() {
		t.Skip("sqlite3 CLI not available")
	}
	db := filepath.Join(t.TempDir(), "claude-mem.db")
	cmd := exec.CommandContext(t.Context(), "sqlite3", db)
	cmd.Stdin = strings.NewReader(testClaudeMemSchema + sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create claude-mem sqlite fixture: %v: %s", err, out)
	}
	return db
}

func TestClaudeMemDBIngestsObservationsAndSummaries(t *testing.T) {
	sql := `
insert into sdk_sessions values (
  'sdk-abc-123',
  'claude-xyz-789',
  '/Users/alice/projects/demo-proj',
  1700000000,
  1700000100,
  1700000200
);

insert into user_prompts values (
  1,
  'sdk-abc-123',
  'how do we parse claude-mem sqlite database',
  1700000010
);

insert into observations values (
  1,
  'sdk-abc-123',
  'SQLite Ingestion Title',
  'Subtitle finding',
  'Observed that observations table holds observations',
  'Detailed observation text body',
  'Fact 1: uses sqlite3 CLI',
  '["internal/sources/claudemem.go", "internal/sources/claude.go"]',
  1700000050
);

insert into session_summaries values (
  1,
  'sdk-abc-123',
  'Build claude-mem ingestion',
  'Investigated schema and tables',
  'Learned observation and summary shapes',
  'Completed implementation of claudemem.go',
  'Next steps write tests',
  'Notes: verified against live schema',
  1700000100
);
`
	db := createTestClaudeMemDB(t, sql)
	t.Setenv("DEJA_CLAUDE_MEM_DB", db)

	if !IsClaudeMemDB(db) {
		t.Fatalf("IsClaudeMemDB(%q) = false, want true", db)
	}
	if !UnderClaudeRoot(db) {
		t.Fatalf("UnderClaudeRoot(%q) = false, want true", db)
	}
	if !slices.Contains(ClaudeFiles(), db) {
		t.Fatalf("ClaudeFiles() does not contain %q", db)
	}

	sessions := LoadClaude()
	var found *model.Session
	for i := range sessions {
		if sessions[i].ID == "cm-sdk-abc-123" {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("LoadClaude() did not return session cm-sdk-abc-123, got: %v", sessions)
	}

	if found.Harness != "claude" {
		t.Errorf("Harness = %q, want %q", found.Harness, "claude")
	}
	if found.Project != "projects/demo-proj" {
		t.Errorf("Project = %q, want %q", found.Project, "projects/demo-proj")
	}
	if found.Title != "Build claude-mem ingestion" {
		t.Errorf("Title = %q, want %q", found.Title, "Build claude-mem ingestion")
	}

	var hasPrompt, hasObservation, hasSummary, hasEdit bool
	for _, m := range found.Messages {
		if m.Role == "user" && strings.Contains(m.Text, "how do we parse claude-mem sqlite database") {
			hasPrompt = true
		}
		if m.Role == "assistant" && strings.Contains(m.Text, "SQLite Ingestion Title") && strings.Contains(m.Text, "Detailed observation text body") {
			hasObservation = true
		}
		if m.Role == "assistant" && strings.Contains(m.Text, "Investigated schema and tables") && strings.Contains(m.Text, "Completed implementation of claudemem.go") {
			hasSummary = true
		}
		if m.Role == RoleEdit && (m.Text == "internal/sources/claudemem.go" || m.Text == "internal/sources/claude.go") {
			hasEdit = true
		}
	}

	if !hasPrompt {
		t.Errorf("missing user prompt in messages: %v", found.Messages)
	}
	if !hasObservation {
		t.Errorf("missing observation assistant message: %v", found.Messages)
	}
	if !hasSummary {
		t.Errorf("missing summary assistant message: %v", found.Messages)
	}
	if !hasEdit {
		t.Errorf("missing RoleEdit message from files_modified: %v", found.Messages)
	}

	// Test incremental / watermark query
	sinceOld, err := ParseClaudeMemDBSince(db, time.Unix(1700000100, 0))
	if err != nil {
		t.Fatalf("ParseClaudeMemDBSince err: %v", err)
	}
	if len(sinceOld) == 0 {
		t.Fatalf("ParseClaudeMemDBSince before last_activity returned 0 sessions")
	}

	sinceNew, err := ParseClaudeMemDBSince(db, time.Unix(1700000500, 0))
	if err != nil {
		t.Fatalf("ParseClaudeMemDBSince err: %v", err)
	}
	if len(sinceNew) != 0 {
		t.Fatalf("ParseClaudeMemDBSince after last_activity returned %d sessions, want 0", len(sinceNew))
	}
}
