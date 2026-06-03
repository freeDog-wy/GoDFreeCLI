package llm

import (
	"context"
)

// Session manages conversation history and provides a simplified API
// for multi-turn chat interactions.
//
// Session wraps a Provider and maintains message history, system prompt,
// and tool definitions. Each call to Send appends the user message and
// streams events; the caller is responsible for consuming the event channel.
//
// The caller owns history extraction via [Session.History]. Session does
// not automatically append assistant responses — use the events to
// reconstruct them, or call [Session.SendAndCollect] for simple cases.
type Session struct {
	provider  Provider
	system    string
	messages  []Message
	tools     []Tool
	maxTokens int
	maxRounds int
}

// SessionOption configures a Session.
type SessionOption func(*Session)

// WithMaxTokens sets the default max_tokens for each Send call.
func WithMaxTokens(n int) SessionOption {
	return func(s *Session) { s.maxTokens = n }
}

// WithMaxRounds sets the default max_rounds for each Send call.
func WithMaxRounds(n int) SessionOption {
	return func(s *Session) { s.maxRounds = n }
}

// NewSession creates a new conversation session.
//
// The provider handles all LLM calls. The system prompt and tools are
// sent with every request. Tools can be nil.
func NewSession(provider Provider, system string, tools []Tool, opts ...SessionOption) *Session {
	s := &Session{
		provider:  provider,
		system:    system,
		tools:     tools,
		maxTokens: 4096,
		maxRounds: 10,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Send sends a user message and returns a channel of streaming events.
//
// The message is automatically appended to the session history before
// sending. The caller is responsible for reading events from the channel.
// After the channel closes (DoneEvent or ErrorEvent), the caller may
// reconstruct the assistant response from the events to manually append
// to history, or use SendAndCollect instead.
func (s *Session) Send(ctx context.Context, text string) <-chan Event {
	s.messages = append(s.messages, NewUserMessage(text))

	return s.provider.StreamChat(ctx, &ChatRequest{
		Model:     "", // provider default
		System:    s.system,
		Messages:  s.messages,
		Tools:     s.tools,
		MaxTokens: s.maxTokens,
		MaxRounds: s.maxRounds,
	})
}

// History returns a copy of the current conversation history.
func (s *Session) History() []Message {
	cp := make([]Message, len(s.messages))
	copy(cp, s.messages)
	return cp
}

// Clear resets the conversation history.
func (s *Session) Clear() {
	s.messages = nil
}

// SetSystem replaces the system prompt.
func (s *Session) SetSystem(prompt string) {
	s.system = prompt
}

// SetTools replaces the available tools.
func (s *Session) SetTools(tools []Tool) {
	s.tools = tools
}
