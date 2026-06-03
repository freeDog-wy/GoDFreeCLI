// Package llm defines the provider-agnostic LLM abstraction layer.
//
// It provides:
//   - A Provider interface for streaming chat completions
//   - An event system for consuming streaming responses
//   - Tool and message type definitions
//   - Hook interfaces for lifecycle extensibility
package llm

import (
	"context"
	"encoding/json"
)

// ────────────────────────────────────────────────────────────
// Event system
// ────────────────────────────────────────────────────────────

// Event is the marker interface for all streaming events.
// Consumers type-switch on the concrete event type.
type Event interface {
	eventMarker()
}

// ── Content events (streaming deltas) ──

// ThinkingEvent carries incremental thinking/reasoning content.
type ThinkingEvent struct {
	Thinking  string // incremental thinking text
	Signature string // thinking signature (may be empty until final delta)
}

func (ThinkingEvent) eventMarker() {}

// TextDeltaEvent carries incremental text content from the assistant.
type TextDeltaEvent struct {
	Text string
}

func (TextDeltaEvent) eventMarker() {}

// ToolCallEvent is emitted when a client-side tool call block is complete.
// It carries the full parsed tool call (not partial JSON).
type ToolCallEvent struct {
	ID    string          // tool use ID for matching results
	Name  string          // tool name
	Input json.RawMessage // complete JSON arguments
}

func (ToolCallEvent) eventMarker() {}

// ToolResultEvent is emitted after a client-side tool has been executed.
type ToolResultEvent struct {
	ID      string // matches ToolCallEvent.ID
	Name    string // tool name
	Result  string // execution result
	IsError bool   // whether the result is an error
}

func (ToolResultEvent) eventMarker() {}

// ServerToolResultEvent is emitted when the API auto-executes a server-side
// tool (web_search, web_fetch, code_execution, etc.) and returns results.
type ServerToolResultEvent struct {
	Index          int
	ToolType       string          // "web_search", "web_fetch", "code_execution"
	WebSearchItems []WebSearchItem // populated for web_search results
	Content        string          // for other server tool results
}

func (ServerToolResultEvent) eventMarker() {}

// WebSearchItem represents a single web search result.
type WebSearchItem struct {
	Title   string
	URL     string
	Snippet string
}

// ── Structural events (block/message boundaries) ──

// ContentBlockStartEvent signals the beginning of a new content block.
type ContentBlockStartEvent struct {
	Index        int
	BlockType    ContentBlockType
	ToolCallID   string // populated for tool_use blocks
	ToolCallName string // populated for tool_use blocks
}

func (ContentBlockStartEvent) eventMarker() {}

// ContentBlockEndEvent signals the end of a content block.
type ContentBlockEndEvent struct {
	Index     int
	BlockType ContentBlockType
}

func (ContentBlockEndEvent) eventMarker() {}

// MessageStartEvent signals the beginning of a new assistant message.
type MessageStartEvent struct {
	Model string
}

func (MessageStartEvent) eventMarker() {}

// MessageEndEvent signals the end of an assistant message.
type MessageEndEvent struct {
	StopReason string // "end_turn", "tool_use", "max_tokens", "stop_sequence", "refusal"
	Usage      Usage
}

func (MessageEndEvent) eventMarker() {}

// ErrorEvent carries an error that occurred during streaming.
// After an ErrorEvent, the channel is closed (no DoneEvent follows).
type ErrorEvent struct {
	Err error
}

func (ErrorEvent) eventMarker() {}

// AskUserEvent is emitted when the model calls the ask_user tool.
// It signals that the provider is pausing the ReAct loop so the
// caller can present the question to the user and resume with a response.
//
// Unlike normal tool calls, ask_user is intercepted before execution —
// the ToolExecutor is never invoked. The stream ends after this event
// and the caller should feed the user's answer back as a tool_result
// content block in the next request.
type AskUserEvent struct {
	ID    string          // tool call ID for matching the user's response
	Name  string          // "ask_user"
	Input json.RawMessage // complete tool call input (contains the question)
}

func (AskUserEvent) eventMarker() {}

// UseSkillEvent is emitted when the model calls the use_skill tool.
// It signals that the provider is pausing the ReAct loop so the
// caller can activate the requested skill (inject prompt, register
// tools) and resume with the enhanced setup.
//
// Unlike ask_user, the caller handles use_skill automatically —
// no user interaction is required.
type UseSkillEvent struct {
	ID    string          // tool call ID
	Input json.RawMessage // tool call input (contains the skill name)
}

func (UseSkillEvent) eventMarker() {}

// DelegateTaskEvent is emitted when the model calls the delegate_task tool.
// The provider pauses the ReAct loop so the caller can spawn a sub-agent
// with limited tools, run it in isolation, and feed the result back.
type DelegateTaskEvent struct {
	ID    string          // tool call ID
	Input json.RawMessage // delegate_task arguments
}

func (DelegateTaskEvent) eventMarker() {}

// DoneEvent signals that the provider has finished all processing
// (including all ReAct rounds). The channel is closed after this event.
type DoneEvent struct{}

func (DoneEvent) eventMarker() {}

// ────────────────────────────────────────────────────────────
// Well-known stop reasons
// ────────────────────────────────────────────────────────────

const (
	StopReasonEndTurn      = "end_turn"
	StopReasonMaxTokens    = "max_tokens"
	StopReasonStopSequence = "stop_sequence"
	StopReasonRefusal      = "refusal"
	StopReasonToolUse      = "tool_use"
	StopReasonAskUser      = "ask_user"
	StopReasonUseSkill     = "use_skill"
	StopReasonDelegate     = "delegate_task"
)

// ────────────────────────────────────────────────────────────
// Enums and supporting types
// ────────────────────────────────────────────────────────────

// ContentBlockType identifies the kind of content block.
type ContentBlockType string

const (
	BlockTypeText             ContentBlockType = "text"
	BlockTypeThinking         ContentBlockType = "thinking"
	BlockTypeToolUse          ContentBlockType = "tool_use"
	BlockTypeServerToolUse    ContentBlockType = "server_tool_use"
	BlockTypeWebSearchResult  ContentBlockType = "web_search_result"
	BlockTypeWebFetchResult   ContentBlockType = "web_fetch_result"
	BlockTypeCodeExecResult   ContentBlockType = "code_execution_result"
	BlockTypeToolResult       ContentBlockType = "tool_result"
)

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// ────────────────────────────────────────────────────────────
// Provider interface
// ────────────────────────────────────────────────────────────

// Provider abstracts an LLM backend (Anthropic, OpenAI, etc.).
type Provider interface {
	// StreamChat sends a chat request and returns a channel of streaming events.
	//
	// The channel receives events as the response streams in. The provider
	// internally handles the ReAct tool-calling loop — the consumer sees
	// multiple rounds of MessageStart/End, tool calls, and results as a single
	// flat stream of events.
	//
	// The channel is closed when all processing completes (after DoneEvent)
	// or when an error occurs (after ErrorEvent).
	//
	// Cancelling the context stops processing.
	StreamChat(ctx context.Context, req *ChatRequest) <-chan Event
}

// ────────────────────────────────────────────────────────────
// Request types
// ────────────────────────────────────────────────────────────

// ChatRequest is a single request to the LLM provider.
type ChatRequest struct {
	Model       string
	System      string         // system prompt
	Messages    []Message      // conversation history
	Tools       []Tool         // available tools for the model to call
	MaxTokens   int            // max output tokens (0 = provider default)
	Temperature *float64       // sampling temperature (nil = provider default)
	MaxRounds   int            // max ReAct rounds (default: 10, 0 = no tool loop)
	Metadata    map[string]any // provider-specific or hook metadata
}

// ────────────────────────────────────────────────────────────
// Tool types
// ────────────────────────────────────────────────────────────

// Tool defines a callable tool for the model.
type Tool struct {
	Name        string
	Description string
	Parameters  *ToolSchema // nil means parameterless tool
}

// ToolSchema describes a tool's JSON Schema for its parameters.
type ToolSchema struct {
	Type       string              // typically "object"
	Properties map[string]Property
	Required   []string
}

// Property is a single property in a ToolSchema.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolExecutor is called by the provider when the model requests a tool call.
//
// Parameters:
//   - ctx: the request context
//   - name: the tool name
//   - input: the JSON arguments (complete, parseable)
//
// Returns the result string. If isError is true, the result is treated as an
// error to the model.
type ToolExecutor func(ctx context.Context, name string, input json.RawMessage) (result string, isError bool)

// ────────────────────────────────────────────────────────────
// Message types
// ────────────────────────────────────────────────────────────

// Role is the sender of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message represents a single message in the conversation.
type Message struct {
	Role    Role
	Content []ContentBlock
}

// ContentBlock is a single block within a message.
type ContentBlock struct {
	Type       ContentBlockType
	Text       string          // for text blocks
	ToolCallID string          // for tool_use blocks
	ToolName   string          // for tool_use blocks
	ToolInput  json.RawMessage // for tool_use blocks
	ToolResult string          // for tool_result blocks
	IsError    bool            // for tool_result blocks
	Data       json.RawMessage // raw block data (for provider-specific pass-through)
}

// ────────────────────────────────────────────────────────────
// Convenience constructors
// ────────────────────────────────────────────────────────────

// NewUserMessage creates a user message with the given text content.
func NewUserMessage(text string) Message {
	return Message{
		Role: RoleUser,
		Content: []ContentBlock{
			{Type: BlockTypeText, Text: text},
		},
	}
}

// NewAssistantMessage creates an assistant message from content blocks.
func NewAssistantMessage(blocks ...ContentBlock) Message {
	return Message{
		Role:    RoleAssistant,
		Content: blocks,
	}
}
