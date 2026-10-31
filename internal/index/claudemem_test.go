package index

import (
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshulcz/deja-vu/internal/search"
)

func TestClaudeMemDBIndexingAndSearch(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
	tmp := t.TempDir()
	setHome(t, tmp)

	db := filepath.Join(tmp, "claude-mem.db")
	t.Setenv("DEJA_CLAUDE_MEM_DB", db)

	stmts := `
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

insert into sdk_sessions values (
  's1', 'c1', '/Users/alice/projects/payment-service',
  1700000000, 1700000100, 1700000200
);

insert into user_prompts values (
  1, 's1', 'payment gateway timeout retry loop', 1700000010
);

insert into observations values (
  1, 's1', 'Payment Gateway Findings', 'Timeout in retry loop',
  'Exponential backoff prevents cascading failure in payment gateway',
  'Circuit breaker observed working', 'Fact: latency dropped',
  '["pkg/pay/gateway.go"]', 1700000050
);

insert into session_summaries values (
  1, 's1', 'Fix payment gateway timeout',
  'Investigated backoff logic',
  'Learned optimal retry window is 200ms',
  'Completed payment circuit breaker',
  'Next steps write integration tests',
  'Notes: production verified', 1700000100
);
`
	c := exec.CommandContext(t.Context(), "sqlite3", db)
	c.Stdin = strings.NewReader(stmts)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("create sqlite fixture: %v %s", err, out)
	}

	indexDir := filepath.Join(tmp, "index")
	if err := Ensure(indexDir, "", false, io.Discard); err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}

	results, err := Search(indexDir, search.Options{Query: "circuit breaker payment", All: true})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("Search returned 0 results for 'circuit breaker payment'")
	}

	s := results[0]
	if s.ID != "cm-s1" {
		t.Errorf("ID = %q, want cm-s1", s.ID)
	}
	if s.Harness != "claude" {
		t.Errorf("Harness = %q, want claude", s.Harness)
	}
}
