package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"math"
)

var DB *sql.DB

func InitDB() error {
	var err error
	DB, err = sql.Open("sqlite3", "./assistant.db?_busy_timeout=5000&_journal_mode=WAL")
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
	// Migrate databases created by the earlier Phase 4 schema.
	rows, err := DB.Query(`PRAGMA table_info(messages)`)
	if err != nil {
		return err
	}
	hasEmbedding := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err = rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "embedding" {
			hasEmbedding = true
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasEmbedding {
		if _, err = DB.Exec(`ALTER TABLE messages ADD COLUMN embedding TEXT`); err != nil {
			return err
		}
	}
	return DB.Ping()
}
func SaveMessage(role, content string, embedding []float64) error {
	var encoded any
	if len(embedding) > 0 {
		b, e := json.Marshal(embedding)
		if e != nil {
			return e
		}
		encoded = string(b)
	}
	_, err := DB.Exec(`INSERT INTO messages(role,content,embedding) VALUES(?,?,?)`, role, content, encoded)
	return err
}
func RecentMessages(limit int) ([]Message, error) {
	rows, err := DB.Query(`SELECT id,role,content,created_at FROM messages ORDER BY id DESC LIMIT ?`, limit)
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
func RelevantMessages(query []float64, limit int) ([]ScoredMessage, error) {
	if len(query) == 0 {
		return nil, nil
	}
	rows, err := DB.Query(`SELECT id,role,content,embedding,created_at FROM messages WHERE embedding IS NOT NULL ORDER BY id DESC LIMIT 500`)
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
	// Small in-memory ranking; no external vector database required.
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].Score > scored[i].Score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}
func MessageCount() (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&count)
	return count, err
}

func SaveSummary(summary string) error {
	if summary == "" {
		return fmt.Errorf("summary is empty")
	}
	_, err := DB.Exec(`INSERT INTO conversation_summaries(summary) VALUES(?)`, summary)
	return err
}
func LatestSummary() (string, error) {
	var s string
	err := DB.QueryRow(`SELECT summary FROM conversation_summaries ORDER BY id DESC LIMIT 1`).Scan(&s)
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
