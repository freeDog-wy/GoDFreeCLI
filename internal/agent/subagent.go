// Package agent provides sub-agent delegation for offloading work
// from the main agent to isolated, short-lived child agents.
package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// Result holds the outcome of a sub-agent run.
type Result struct {
	// Output is the final text content produced by the sub-agent.
	Output string
	// Rounds is the number of ReAct rounds executed.
	Rounds int
	// TokensUsed estimates total tokens consumed (input + output).
	TokensUsed int64
	// Error is non-nil if the sub-agent failed.
	Error error
}

// Config configures a sub-agent run.
type Config struct {
	// Description is the task for the sub-agent.
	Description string
	// AllTools is the full tool list the main agent has access to.
	// The sub-agent only uses names matching ToolFilter.
	AllTools []llm.Tool
	// ToolFilter is a set of allowed tool names. If empty, all tools are available.
	ToolFilter map[string]bool
	// MaxRounds limits ReAct rounds (default 5).
	MaxRounds int
	// Model is the model to use (e.g. "claude-sonnet-4-6").
	Model string
}

// Run spawns an isolated sub-agent, runs it to completion, and returns
// the result. It is safe to call from multiple goroutines.
//
// The sub-agent uses a fresh message history containing only the task
// description, and a filtered tool set. No state leaks between the
// main agent and the sub-agent.
func Run(ctx context.Context, provider llm.Provider, cfg Config) Result {
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 5
	}
	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-6"
	}

	// Build the filtered tool list for the sub-agent
	tools := filterTools(cfg.AllTools, cfg.ToolFilter)

	// System prompt for sub-agents
	system := []string{
		"You are a sub-agent performing a delegated task.",
		"Work efficiently: use tools when helpful, but don't over-investigate.",
		"When you have a complete answer, respond directly in plain text.",
		"Your final message is the only output the main agent will see.",
		"Do NOT use delegate_task, ask_user, use_skill, or browser tools.",
	}
	systemPrompt := strings.Join(system, "\n")

	messages := []llm.Message{
		llm.NewUserMessage(fmt.Sprintf("Task: %s", cfg.Description)),
	}

	req := &llm.ChatRequest{
		Model:     cfg.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  messages,
		Tools:     tools,
		MaxRounds: cfg.MaxRounds,
	}

	// Collect events from the sub-agent's stream
	var (
		output    strings.Builder
		rounds    int
		totalIn   int64
		totalOut  int64
		lastError error
		mu        sync.Mutex // protects counters during concurrent access
	)

	ch := provider.StreamChat(ctx, req)
	for evt := range ch {
		switch e := evt.(type) {
		case llm.TextDeltaEvent:
			mu.Lock()
			output.WriteString(e.Text)
			mu.Unlock()

		case llm.MessageStartEvent:
			mu.Lock()
			rounds++
			mu.Unlock()

		case llm.MessageEndEvent:
			mu.Lock()
			totalIn += e.Usage.InputTokens
			totalOut += e.Usage.OutputTokens
			mu.Unlock()

		case llm.ErrorEvent:
			lastError = e.Err
		}
	}

	if lastError != nil {
		return Result{
			Output:     output.String(),
			Rounds:     rounds,
			TokensUsed: totalIn + totalOut,
			Error:      lastError,
		}
	}

	return Result{
		Output:     strings.TrimSpace(output.String()),
		Rounds:     rounds,
		TokensUsed: totalIn + totalOut,
	}
}

// SubmitResult formats a sub-agent Result as a tool result string
// to feed back to the main agent.
func SubmitResult(r Result) string {
	if r.Error != nil {
		return fmt.Sprintf("[sub-agent error: %v]\nPartial output: %s", r.Error, r.Output)
	}
	return fmt.Sprintf("[sub-agent completed in %d rounds, ~%d tokens]\n%s", r.Rounds, r.TokensUsed, r.Output)
}

// filterTools returns tools whose names are in the allowlist.
// If allowlist is nil or empty, all tools are returned.
func filterTools(all []llm.Tool, allowlist map[string]bool) []llm.Tool {
	if len(allowlist) == 0 {
		return all
	}
	out := make([]llm.Tool, 0, len(allowlist))
	for _, t := range all {
		if allowlist[t.Name] {
			out = append(out, t)
		}
	}
	return out
}
