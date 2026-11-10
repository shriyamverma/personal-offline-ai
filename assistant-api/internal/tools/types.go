package tools

type ExecuteParams struct {
	Path     string
	Command  string
	Content  string
	Approved bool
}

type ExecuteResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}
