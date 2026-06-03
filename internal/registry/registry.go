// Package registry provides a unified tool registry that aggregates
// tool definitions and executors from multiple sources (built-in, MCP, etc).
//
// The registry implements llm.ToolExecutor so it can be passed directly
// to the provider as the default executor.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// Registry holds all registered tools and their executors. It is safe for
// concurrent use after creation (all mutations happen during initialization).
type Registry struct {
	mu       sync.RWMutex
	tools    []llm.Tool
	execMap  map[string]llm.ToolExecutor
}

// New creates an empty tool registry.
func New() *Registry {
	return &Registry{
		execMap: make(map[string]llm.ToolExecutor),
	}
}

// Register adds a tool + executor pair. Returns an error if a tool with
// the same name is already registered (duplicate tool names are rejected).
func (r *Registry) Register(t llm.Tool, exec llm.ToolExecutor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.execMap[t.Name]; ok {
		return fmt.Errorf("tool %q is already registered", t.Name)
	}
	r.tools = append(r.tools, t)
	r.execMap[t.Name] = exec
	return nil
}

// RegisterAll adds multiple tool + executor pairs. If any tool name
// conflicts, it returns an error and no tools are added.
func (r *Registry) RegisterAll(tools []llm.Tool, execMap map[string]llm.ToolExecutor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for conflicts first
	for _, t := range tools {
		if _, ok := r.execMap[t.Name]; ok {
			return fmt.Errorf("tool %q is already registered", t.Name)
		}
	}

	for _, t := range tools {
		exec, ok := execMap[t.Name]
		if !ok {
			return fmt.Errorf("executor missing for tool %q", t.Name)
		}
		r.tools = append(r.tools, t)
		r.execMap[t.Name] = exec
	}
	return nil
}

// MustRegister is like Register but panics on duplicate. Useful for
// built-in tools that should never conflict.
func (r *Registry) MustRegister(t llm.Tool, exec llm.ToolExecutor) {
	if err := r.Register(t, exec); err != nil {
		panic(err)
	}
}

// Tools returns a snapshot of all registered tool definitions.
func (r *Registry) Tools() []llm.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := make([]llm.Tool, len(r.tools))
	copy(cp, r.tools)
	return cp
}

// Execute dispatches a tool call to the registered executor.
// Implements llm.ToolExecutor so the registry can be used directly
// as the provider's default executor.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, bool) {
	r.mu.RLock()
	exec, ok := r.execMap[name]
	r.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("unknown tool: %s", name), true
	}
	return exec(ctx, name, input)
}
