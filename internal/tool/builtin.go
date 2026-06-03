// Package tool provides built-in tool definitions and helpers
// for the GoDFreeCLI agent framework.
//
// Built-in tools are special tools that are intercepted by the provider
// and not executed through the normal ToolExecutor path. Instead, they
// produce dedicated events that the caller handles at a higher level.
package tool

import "github.com/freeDog-wy/GoDFreeCLI/internal/llm"

// ── Well-known tool names ──

const (
	// AskUserName is the canonical tool name for asking the user a question.
	// When the model calls this tool, the provider emits an [llm.AskUserEvent]
	// instead of executing it via the ToolExecutor.
	AskUserName = "ask_user"
)

// ── Tool definitions ──

// AskUserTool returns the built-in ask_user tool definition.
func AskUserTool() llm.Tool {
	return llm.Tool{
		Name:        AskUserName,
		Description: "Ask the user a question when you need clarification, confirmation, or additional information to proceed. Use this tool when you are uncertain about the user's intent, need to confirm a destructive action, or require extra details to complete a task accurately.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"question": {
					Type:        "string",
					Description: "The question to ask the user. Be specific and clear about what information you need.",
				},
				"context": {
					Type:        "string",
					Description: "Optional context explaining why this information is needed, to help the user understand the request.",
				},
			},
			Required: []string{"question"},
		},
	}
}

// AskUserInput is a typed representation of the ask_user tool's input.
type AskUserInput struct {
	Question string `json:"question"`
	Context  string `json:"context,omitempty"`
}

// ── Convenience: all tools ────────────────────────────────

// AllTools returns all built-in tools enabled by the given ToolSet.
func AllTools(ts ToolSet) []llm.Tool {
	var tools []llm.Tool
	if ts.FileOps {
		tools = append(tools, FileTools()...)
	}
	if ts.Bash {
		tools = append(tools, BashTool())
	}
	if ts.UserInput {
		tools = append(tools, AskUserTool())
	}
	// delegate_task is always available (intercepted by provider)
	tools = append(tools, DelegateTaskTool())
	return tools
}
