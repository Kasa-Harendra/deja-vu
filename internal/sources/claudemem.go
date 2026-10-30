package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vshulcz/deja-vu/internal/model"
)

// ClaudeMemDBPath returns the path to the claude-mem SQLite database.
// Honors DEJA_CLAUDE_MEM_DB, falling back to ~/.claude-mem/claude-mem.db.
func ClaudeMemDBPath() string {
	if p := os.Getenv("DEJA_CLAUDE_MEM_DB"); p != "" {
		return p
	}
	return filepath.Join(Home(), ".claude-mem", "claude-mem.db")
}

// IsClaudeMemDB reports whether path matches the configured claude-mem database.
func IsClaudeMemDB(path string) bool {
	db := ClaudeMemDBPath()
	if db == "" {
		return false
	}
	return filepath.Clean(path) == filepath.Clean(db)
}

// ParseClaudeMemDB reads every session in the claude-mem SQLite database.
func ParseClaudeMemDB(db string) ([]model.Session, error) {
	return ParseClaudeMemDBSince(db, time.Time{})
}

// ParseClaudeMemDBSince reads the sessions in the claude-mem SQLite database touched after t.
func ParseClaudeMemDBSince(db string, t time.Time) ([]model.Session, error) {
	if fi, err := os.Stat(db); err != nil || fi.Size() == 0 {
		return nil, nil
	}
	where := ""
	if !t.IsZero() {
		cutoff := t.Add(-time.Second)
		where = fmt.Sprintf(" where (%s or %s or %s)",
			newerThanEpoch("s.last_activity_epoch", cutoff),
			newerThanEpoch("s.completed_at_epoch", cutoff),
			newerThanEpoch("s.created_at_epoch", cutoff),
		)
	}
	return parseClaudeMemDBWhere(db, where)
}

func parseClaudeMemDBWhere(db, where string) ([]model.Session, error) {
	out, err := sqliteOutput(db, "select count(*) from sqlite_master where type='table' and name='sdk_sessions'")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(out)) == "0" {
		return nil, nil
	}

	qSessions := "select json_object('sdk_session_id',cast(s.sdk_session_id as text)," +
		"'claude_session_id',cast(coalesce(s.claude_session_id,'') as text)," +
		"'project',cast(coalesce(s.project,'') as text)," +
		"'created_at_epoch',s.created_at_epoch," +
		"'completed_at_epoch',coalesce(s.completed_at_epoch,0)," +
		"'last_activity_epoch',coalesce(s.last_activity_epoch,0)) " +
		"from sdk_sessions s" + where + " order by s.created_at_epoch"
	outSessions, err := sqliteOutput(db, qSessions)
	if err != nil {
		return nil, err
	}

	type sessionRow struct {
		SDKSessionID      string `json:"sdk_session_id"`
		ClaudeSessionID   string `json:"claude_session_id"`
		Project           string `json:"project"`
		CreatedAtEpoch    int64  `json:"created_at_epoch"`
		CompletedAtEpoch  int64  `json:"completed_at_epoch"`
		LastActivityEpoch int64  `json:"last_activity_epoch"`
	}
	sRows, err := sqliteObjects[sessionRow](outSessions)
	if err != nil {
		return nil, err
	}
	if len(sRows) == 0 {
		return nil, nil
	}

	sessionsByID := make(map[string]*model.Session, len(sRows))
	var sessionOrder []string
	for _, r := range sRows {
		if r.SDKSessionID == "" {
			continue
		}
		proj := projectName(r.Project)
		if proj == "" || proj == "-" {
			if r.Project != "" {
				proj = filepath.Base(r.Project)
			}
		}
		if proj == "" {
			proj = "claude"
		}
		started := unixGuess(r.CreatedAtEpoch)
		updated := unixGuess(r.LastActivityEpoch)
		if updated.IsZero() {
			updated = unixGuess(r.CompletedAtEpoch)
		}
		if updated.IsZero() {
			updated = started
		}
		s := &model.Session{
			Harness: "claude",
			ID:      "cm-" + r.SDKSessionID,
			Project: proj,
			Path:    db,
			Started: started,
			Updated: updated,
		}
		sessionsByID[r.SDKSessionID] = s
		sessionOrder = append(sessionOrder, r.SDKSessionID)
	}

	hasPrompts, _ := sqliteOutput(db, "select count(*) from sqlite_master where type='table' and name='user_prompts'")
	if strings.TrimSpace(string(hasPrompts)) != "0" {
		qPrompts := "select json_object('sdk_session_id',cast(u.sdk_session_id as text)," +
			"'prompt_text',cast(coalesce(u.prompt_text,'') as text)," +
			"'created_at_epoch',u.created_at_epoch) " +
			"from user_prompts u join sdk_sessions s on u.sdk_session_id = s.sdk_session_id" + where + " order by u.created_at_epoch"
		outPrompts, err := sqliteOutput(db, qPrompts)
		if err == nil {
			type promptRow struct {
				SDKSessionID   string `json:"sdk_session_id"`
				PromptText     string `json:"prompt_text"`
				CreatedAtEpoch int64  `json:"created_at_epoch"`
			}
			pRows, _ := sqliteObjects[promptRow](outPrompts)
			for _, pr := range pRows {
				s := sessionsByID[pr.SDKSessionID]
				if s == nil {
					continue
				}
				txt := strings.TrimSpace(pr.PromptText)
				if txt == "" {
					continue
				}
				at := unixGuess(pr.CreatedAtEpoch)
				s.Messages = append(s.Messages, model.Message{
					Role: "user",
					Text: txt,
					Time: at,
				})
			}
		}
	}

	hasObs, _ := sqliteOutput(db, "select count(*) from sqlite_master where type='table' and name='observations'")
	if strings.TrimSpace(string(hasObs)) != "0" {
		qObs := "select json_object('sdk_session_id',cast(o.sdk_session_id as text)," +
			"'title',cast(coalesce(o.title,'') as text)," +
			"'subtitle',cast(coalesce(o.subtitle,'') as text)," +
			"'narrative',cast(coalesce(o.narrative,'') as text)," +
			"'text',cast(coalesce(o.text,'') as text)," +
			"'facts',cast(coalesce(o.facts,'') as text)," +
			"'files_modified',cast(coalesce(o.files_modified,'') as text)," +
			"'created_at_epoch',o.created_at_epoch) " +
			"from observations o join sdk_sessions s on o.sdk_session_id = s.sdk_session_id" + where + " order by o.created_at_epoch"
		outObs, err := sqliteOutput(db, qObs)
		if err == nil {
			type obsRow struct {
				SDKSessionID   string `json:"sdk_session_id"`
				Title          string `json:"title"`
				Subtitle       string `json:"subtitle"`
				Narrative      string `json:"narrative"`
				Text           string `json:"text"`
				Facts          string `json:"facts"`
				FilesModified  string `json:"files_modified"`
				CreatedAtEpoch int64  `json:"created_at_epoch"`
			}
			oRows, _ := sqliteObjects[obsRow](outObs)
			for _, orow := range oRows {
				s := sessionsByID[orow.SDKSessionID]
				if s == nil {
					continue
				}
				at := unixGuess(orow.CreatedAtEpoch)
				var parts []string
				for _, field := range []string{orow.Title, orow.Subtitle, orow.Narrative, orow.Text, orow.Facts} {
					field = strings.TrimSpace(field)
					if field != "" {
						parts = append(parts, field)
					}
				}
				if len(parts) > 0 {
					s.Messages = append(s.Messages, model.Message{
						Role: "assistant",
						Text: strings.Join(parts, "\n"),
						Time: at,
					})
				}
				if fm := strings.TrimSpace(orow.FilesModified); fm != "" && IndexEdits() {
					var files []string
					if strings.HasPrefix(fm, "[") {
						_ = json.Unmarshal([]byte(fm), &files)
					}
					if len(files) == 0 {
						for _, line := range strings.Split(fm, "\n") {
							for _, p := range strings.Split(line, ",") {
								p = strings.TrimSpace(p)
								if p != "" && p != "[" && p != "]" {
									files = append(files, p)
								}
							}
						}
					}
					for _, f := range files {
						f = strings.TrimSpace(f)
						if f != "" {
							s.Messages = append(s.Messages, model.Message{
								Role: RoleEdit,
								Text: f,
								Time: at,
							})
						}
					}
				}
			}
		}
	}

	hasSum, _ := sqliteOutput(db, "select count(*) from sqlite_master where type='table' and name='session_summaries'")
	if strings.TrimSpace(string(hasSum)) != "0" {
		qSum := "select json_object('sdk_session_id',cast(m.sdk_session_id as text)," +
			"'request',cast(coalesce(m.request,'') as text)," +
			"'investigated',cast(coalesce(m.investigated,'') as text)," +
			"'learned',cast(coalesce(m.learned,'') as text)," +
			"'completed',cast(coalesce(m.completed,'') as text)," +
			"'next_steps',cast(coalesce(m.next_steps,'') as text)," +
			"'notes',cast(coalesce(m.notes,'') as text)," +
			"'created_at_epoch',m.created_at_epoch) " +
			"from session_summaries m join sdk_sessions s on m.sdk_session_id = s.sdk_session_id" + where + " order by m.created_at_epoch"
		outSum, err := sqliteOutput(db, qSum)
		if err == nil {
			type sumRow struct {
				SDKSessionID   string `json:"sdk_session_id"`
				Request        string `json:"request"`
				Investigated   string `json:"investigated"`
				Learned        string `json:"learned"`
				Completed      string `json:"completed"`
				NextSteps      string `json:"next_steps"`
				Notes          string `json:"notes"`
				CreatedAtEpoch int64  `json:"created_at_epoch"`
			}
			sumRows, _ := sqliteObjects[sumRow](outSum)
			for _, sr := range sumRows {
				s := sessionsByID[sr.SDKSessionID]
				if s == nil {
					continue
				}
				if s.Title == "" && strings.TrimSpace(sr.Request) != "" {
					s.Title = strings.TrimSpace(sr.Request)
				}
				at := unixGuess(sr.CreatedAtEpoch)
				var parts []string
				for _, field := range []string{sr.Investigated, sr.Learned, sr.Completed, sr.NextSteps, sr.Notes} {
					field = strings.TrimSpace(field)
					if field != "" {
						parts = append(parts, field)
					}
				}
				if len(parts) > 0 {
					s.Messages = append(s.Messages, model.Message{
						Role: "assistant",
						Text: strings.Join(parts, "\n"),
						Time: at,
					})
				}
			}
		}
	}

	var result []model.Session
	for _, id := range sessionOrder {
		s := sessionsByID[id]
		if s == nil || len(s.Messages) == 0 {
			continue
		}
		sort.SliceStable(s.Messages, func(i, j int) bool {
			return s.Messages[i].Time.Before(s.Messages[j].Time)
		})
		if s.Title == "" {
			for _, m := range s.Messages {
				if m.Role == "user" && m.Text != "" {
					firstLine, _, _ := strings.Cut(m.Text, "\n")
					s.Title = strings.TrimSpace(firstLine)
					break
				}
			}
		}
		if s.Started.IsZero() && len(s.Messages) > 0 {
			s.Started = s.Messages[0].Time
		}
		if s.Updated.IsZero() && len(s.Messages) > 0 {
			s.Updated = s.Messages[len(s.Messages)-1].Time
		}
		if s.Updated.Before(s.Started) {
			s.Updated = s.Started
		}
		result = append(result, *s)
	}
	return result, nil
}
