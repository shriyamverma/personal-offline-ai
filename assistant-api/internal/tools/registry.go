package tools

import (
	"fmt"
	"sort"
	"sync"
)

var (
	registryMu sync.RWMutex
	Registry   = map[string]Tool{}
)

func Register(tool Tool) {
	if tool == nil || tool.Name() == "" {
		panic("cannot register nil tool or empty tool name")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := Registry[tool.Name()]; exists {
		panic(fmt.Sprintf("tool already registered: %s", tool.Name()))
	}
	Registry[tool.Name()] = tool
}

func Get(name string) (Tool, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	t, ok := Registry[name]
	return t, ok
}

func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(Registry))
	for name := range Registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
