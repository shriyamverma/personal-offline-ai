package tools

var Registry = map[string]Tool{}

func Register(tool Tool) {
	Registry[tool.Name()] = tool
}