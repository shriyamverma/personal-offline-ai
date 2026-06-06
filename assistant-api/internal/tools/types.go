package tools

type ExecuteParams struct {
	Path    string
	Command string
}

type ExecuteResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}