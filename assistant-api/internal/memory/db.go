package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"math"
	"os"
	"sort"
)

var DB *sql.DB

func InitDB() error {
	path := os.Getenv("DATABASE_PATH")
	if path == "" {
		path = "./assistant.db"
	}
	return Open(path)
}

func Open(path string) error {
	var err error
	DB, err = sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return err
	}
	DB.SetMaxOpenConns(1)
	statements := []string{
		`CREATE TABLE IF NOT EXISTS messages(id INTEGER PRIMARY KEY AUTOINCREMENT,role TEXT NOT NULL,content TEXT NOT NULL,embedding TEXT,created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS conversation_summaries(id INTEGER PRIMARY KEY AUTOINCREMENT,summary TEXT NOT NULL,created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
	}
	for _, q := range statements {
		if _, err = DB.Exec(q); err != nil {
			return err
		}
	}
	// Add conversation scope without discarding existing Phase 4/5 history.
	for _, table := range []string{"messages", "conversation_summaries"} {
		rows, err := DB.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			return err
		}
		columns := map[string]bool{}
		for rows.Next() {
			var cid, nn, pk int
			var name, typ string
			var def any
			if err := rows.Scan(&cid, &name, &typ, &nn, &def, &pk); err != nil {
				rows.Close()
				return err
			}
			columns[name] = true
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			return rowErr
		}
		if table == "messages" && !columns["embedding"] {
			if _, err := DB.Exec("ALTER TABLE messages ADD COLUMN embedding TEXT"); err != nil {
				return err
			}
		}
		if !columns["conversation_id"] {
			if _, err := DB.Exec("ALTER TABLE " + table + " ADD COLUMN conversation_id TEXT NOT NULL DEFAULT 'legacy'"); err != nil {
				return err
			}
		}
	}
	return initConversations()
}

func SaveMessage(conversationID, role, content string, embedding []float64) error {
	var encoded any
	if len(embedding) > 0 {
		b, e := json.Marshal(embedding)
		if e != nil {
			return e
		}
		encoded = string(b)
	}
	_, err := DB.Exec(`INSERT INTO messages(conversation_id,role,content,embedding) VALUES(?,?,?,?)`, conversationID, role, content, encoded)
	return err
}
func RecentMessages(conversationID string, limit int) ([]Message, error) {
	rows, err := DB.Query(`SELECT id,role,content,created_at FROM messages WHERE conversation_id=? ORDER BY id DESC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err = rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}
func RelevantMessages(conversationID string, query []float64, limit int) ([]ScoredMessage, error) {
	if len(query) == 0 {
		return nil, nil
	}
	rows, err := DB.Query(`SELECT id,role,content,embedding,created_at FROM messages WHERE conversation_id=? AND embedding IS NOT NULL ORDER BY id DESC LIMIT 500`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scored []ScoredMessage
	for rows.Next() {
		var m Message
		var raw string
		if err = rows.Scan(&m.ID, &m.Role, &m.Content, &raw, &m.CreatedAt); err != nil {
			return nil, err
		}
		var v []float64
		if json.Unmarshal([]byte(raw), &v) != nil {
			continue
		}
		if len(v) != len(query) {
			continue
		}
		scored = append(scored, ScoredMessage{Message: m, Score: cosine(query, v)})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}
func MessageCount(conversationID string) (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE conversation_id=?`, conversationID).Scan(&count)
	return count, err
}

func SaveSummary(conversationID, summary string) error {
	if summary == "" {
		return fmt.Errorf("summary is empty")
	}
	_, err := DB.Exec(`INSERT INTO conversation_summaries(conversation_id,summary) VALUES(?,?)`, conversationID, summary)
	return err
}
func LatestSummary(conversationID string) (string, error) {
	var s string
	err := DB.QueryRow(`SELECT summary FROM conversation_summaries WHERE conversation_id=? ORDER BY id DESC LIMIT 1`, conversationID).Scan(&s)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return s, err
}
func cosine(a, b []float64) float64 {
	var dot, aa, bb float64
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}
