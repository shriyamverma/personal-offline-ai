package tools

type Tool interface {
	Name() string
	Execute(params ExecuteParams) ExecuteResult
}