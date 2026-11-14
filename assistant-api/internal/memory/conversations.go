package memory

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type Conversation struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
type RunRecord struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	Status         string          `json:"status"`
	State          json.RawMessage `json:"state"`
}

var ErrConflict = errors.New("conversation has an active run or approval already handled")

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func initConversations() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS conversations(id TEXT PRIMARY KEY,title TEXT NOT NULL,model TEXT NOT NULL,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT OR IGNORE INTO conversations(id,title,model) SELECT 'legacy','Imported history','qwen2.5:3b' WHERE EXISTS(SELECT 1 FROM messages WHERE conversation_id='legacy')`,
		`CREATE INDEX IF NOT EXISTS messages_conversation ON messages(conversation_id,id)`,
		`CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY,conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,status TEXT NOT NULL,state TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS one_active_run ON runs(conversation_id) WHERE status IN ('running','awaiting_approval')`,
		`UPDATE runs SET status='interrupted' WHERE status='running'`,
	} {
		if _, err := DB.Exec(q); err != nil {
			return err
		}
	}
	return nil
}
func CreateConversation(title, model string) (Conversation, error) {
	if strings.TrimSpace(title) == "" {
		title = "New conversation"
	}
	id := NewID()
	_, err := DB.Exec(`INSERT INTO conversations(id,title,model) VALUES(?,?,?)`, id, title, model)
	if err != nil {
		return Conversation{}, err
	}
	return GetConversation(id)
}
func GetConversation(id string) (Conversation, error) {
	var c Conversation
	err := DB.QueryRow(`SELECT id,title,model,created_at,updated_at FROM conversations WHERE id=?`, id).Scan(&c.ID, &c.Title, &c.Model, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}
func ListConversations() ([]Conversation, error) {
	rows, err := DB.Query(`SELECT id,title,model,created_at,updated_at FROM conversations ORDER BY updated_at DESC,rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.Model, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func UpdateConversation(id, title, model string) error {
	_, err := DB.Exec(`UPDATE conversations SET title=?,model=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, title, model, id)
	return err
}
func DeleteConversation(id string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM runs WHERE conversation_id=? AND status='running'`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrConflict
	}
	for _, q := range []string{`DELETE FROM messages WHERE conversation_id=?`, `DELETE FROM conversation_summaries WHERE conversation_id=?`, `DELETE FROM conversations WHERE id=?`} {
		if _, err = tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func Messages(id string) ([]Message, error) {
	rows, err := DB.Query(`SELECT id,role,content,created_at FROM messages WHERE conversation_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func CreateRun(r RunRecord) error {
	_, err := DB.Exec(`INSERT INTO runs(id,conversation_id,status,state) VALUES(?,?,?,?)`, r.ID, r.ConversationID, r.Status, string(r.State))
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrConflict
	}
	return err
}
func SaveRun(r RunRecord) error {
	_, err := DB.Exec(`UPDATE runs SET status=?,state=? WHERE id=?`, r.Status, string(r.State), r.ID)
	return err
}
func Runs(id string) ([]RunRecord, error) {
	rows, err := DB.Query(`SELECT id,conversation_id,status,state FROM runs WHERE conversation_id=? ORDER BY rowid`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunRecord{}
	for rows.Next() {
		var r RunRecord
		var state string
		if err := rows.Scan(&r.ID, &r.ConversationID, &r.Status, &state); err != nil {
			return nil, err
		}
		r.State = json.RawMessage(state)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ClaimApproval atomically consumes the pending approval. A replay cannot execute it twice.
func ClaimApproval(id, approvalID string) (RunRecord, error) {
	tx, err := DB.Begin()
	if err != nil {
		return RunRecord{}, err
	}
	defer tx.Rollback()
	var r RunRecord
	var state string
	err = tx.QueryRow(`SELECT id,conversation_id,status,state FROM runs WHERE id=?`, id).Scan(&r.ID, &r.ConversationID, &r.Status, &state)
	if err != nil {
		return r, err
	}
	var pending struct {
		ApprovalID string `json:"approval_id"`
	}
	if err := json.Unmarshal([]byte(state), &pending); err != nil {
		return r, err
	}
	if approvalID == "" || approvalID != pending.ApprovalID {
		return r, ErrConflict
	}
	if r.Status != "awaiting_approval" {
		return r, ErrConflict
	}
	result, err := tx.Exec(`UPDATE runs SET status='running' WHERE id=? AND status='awaiting_approval'`, id)
	if err != nil {
		return r, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return r, ErrConflict
	}
	r.State = json.RawMessage(state)
	r.Status = "running"
	return r, tx.Commit()
}
func IsNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }
