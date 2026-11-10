package memory

type Message struct {
	ID        int64  `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at,omitempty"`
}
type ScoredMessage struct {
	Message
	Score float64 `json:"score"`
}
