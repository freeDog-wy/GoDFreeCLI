package tool

import "github.com/freeDog-wy/GoDFreeCLI/internal/llm"

// DelegateTaskName is the canonical tool name for delegating a sub-task
// to an isolated sub-agent.
const DelegateTaskName = "delegate_task"

// DelegateTaskTool returns the delegate_task tool definition.
//
// When called, the provider intercepts it and emits a DelegateTaskEvent.
// The consumer spawns an isolated sub-agent with the specified tools,
// runs it to completion, and feeds the result back as a tool_result.
func DelegateTaskTool() llm.Tool {
	return llm.Tool{
		Name:        DelegateTaskName,
		Description: "Delegate a sub-task to an isolated sub-agent that runs independently and returns a summary result. Use this for parallelizable or context-heavy work: searching across many files, reviewing a large codebase, researching a topic, or any task where tool-call noise would bloat the main conversation. The sub-agent has its own fresh context and returns only its final answer.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"description": {
					Type:        "string",
					Description: "Clear, self-contained description of the task. Include all necessary context, constraints, and expected output format. The sub-agent has no memory of the main conversation.",
				},
				"tools": {
					Type:        "string",
					Description: "Comma-separated list of tool names the sub-agent can use, e.g. 'read_file,search_content,list_files'. Leave empty or omit to give the sub-agent the same tools as the main agent. Use this to limit scope and reduce token usage.",
				},
				"max_rounds": {
					Type:        "integer",
					Description: "Maximum ReAct rounds for the sub-agent. Default: 5. Lower values save tokens but may prevent complex tasks from completing.",
				},
			},
			Required: []string{"description"},
		},
	}
}

// DelegateTaskInput is a typed representation of delegate_task arguments.
type DelegateTaskInput struct {
	Description string `json:"description"`
	Tools       string `json:"tools,omitempty"` // comma-separated tool names
	MaxRounds   int    `json:"max_rounds,omitempty"`
}
