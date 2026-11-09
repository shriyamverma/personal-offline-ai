package tools

import "fmt"

var Registry = map[string]Tool{}

func Register(tool Tool) {
	if tool == nil {
		panic("cannot register a nil tool")
	}

	name := tool.Name()
	if name == "" {
		panic("cannot register a tool with an empty name")
	}

	if _, exists := Registry[name]; exists {
		panic(fmt.Sprintf("tool already registered: %s", name))
	}

	Registry[name] = tool
}

func Get(name string) (Tool, bool) {
	tool, exists := Registry[name]
	return tool, exists
}

func List() []string {
	names := make([]string, 0, len(Registry))

	for name := range Registry {
		names = append(names, name)
	}

	return names
}
