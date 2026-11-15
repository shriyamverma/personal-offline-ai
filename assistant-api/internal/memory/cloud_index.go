package memory

// InitCloudIndex adds an installation namespace and an idempotent indexing
// checkpoint. Missing checkpoints form a durable retry queue after API outages.
func InitCloudIndex() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS memory_settings(key TEXT PRIMARY KEY,value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cloud_index(message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,index_name TEXT NOT NULL,PRIMARY KEY(message_id,index_name))`,
	} {
		if _, err := DB.Exec(q); err != nil {
			return err
		}
	}
	_, err := DB.Exec(`INSERT OR IGNORE INTO memory_settings(key,value) VALUES('store_id',?)`, NewID())
	return err
}
func StoreID() (string, error) {
	var id string
	err := DB.QueryRow(`SELECT value FROM memory_settings WHERE key='store_id'`).Scan(&id)
	return id, err
}
func UnindexedMessages(conversation, index string, limit int) ([]Message, error) {
	rows, err := DB.Query(`SELECT m.id,m.role,m.content,m.created_at FROM messages m WHERE m.conversation_id=? AND NOT EXISTS(SELECT 1 FROM cloud_index i WHERE i.message_id=m.id AND i.index_name=?) ORDER BY m.id DESC LIMIT ?`, conversation, index, limit)
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
func MarkIndexed(index string, ids []int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO cloud_index(message_id,index_name) VALUES(?,?)`, id, index); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func MessageInConversation(conversation string, id int64) (Message, error) {
	var m Message
	err := DB.QueryRow(`SELECT id,role,content,created_at FROM messages WHERE conversation_id=? AND id=?`, conversation, id).Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt)
	return m, err
}

func ResetCloudIndex(index string) error {
	_, err := DB.Exec(`DELETE FROM cloud_index WHERE index_name=?`, index)
	return err
}
