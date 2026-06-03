package tool

import "github.com/freeDog-wy/GoDFreeCLI/internal/llm"

// ── Bash tool name ──

const (
	// BashName is the canonical tool name for executing shell commands.
	BashName = "execute_bash"
)

// ── execute_bash ──────────────────────────────────────────

// BashTool returns the execute_bash tool definition.
//
// The model uses this to run shell commands. The executor is
// responsible for sandbox enforcement, timeout management, and
// output capture. Commands that mutate the filesystem (write,
// delete, move) should require user confirmation.
func BashTool() llm.Tool {
	return llm.Tool{
		Name:        BashName,
		Description: "Executes a shell command and returns its output (stdout + stderr) along with the exit code. Use this for running build commands, tests, git operations, package managers, and other CLI tools. The command runs in the project root directory unless overridden. Long-running commands are terminated after a timeout.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"command": {
					Type:        "string",
					Description: "The shell command to execute. Use PowerShell syntax on Windows, bash syntax on Unix. For git, npm, go, and other common tools, standard CLI syntax works across platforms. Example: 'go build ./...'",
				},
				"workdir": {
					Type:        "string",
					Description: "Optional. Working directory for the command. Defaults to the project root.",
				},
				"timeout_ms": {
					Type:        "integer",
					Description: "Optional. Timeout in milliseconds. Default: 120000 (2 minutes). Max: 600000 (10 minutes). Use for commands that might hang.",
				},
				"description": {
					Type:        "string",
					Description: "A short description (5-10 words) of what this command does. Used for logging and audit trails. Example: 'Build the Go project', 'Run unit tests', 'Install dependencies'.",
				},
			},
			Required: []string{"command"},
		},
	}
}

// BashInput is a typed representation of execute_bash arguments.
type BashInput struct {
	Command     string `json:"command"`
	Workdir     string `json:"workdir,omitempty"`
	TimeoutMs   int    `json:"timeout_ms,omitempty"`
	Description string `json:"description,omitempty"`
}

// BashResult is a structured result from executing a bash command.
// Use this when constructing the executor's return value so the model
// can interpret success/failure reliably.
type BashResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out,omitempty"`
}
