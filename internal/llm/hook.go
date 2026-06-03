package llm

import (
	"context"
	"encoding/json"
)

// Hooks defines lifecycle callbacks for the LLM provider pipeline.
// Each hook is optional — set to nil to skip. Hooks are invoked
// synchronously, so they should return quickly.
//
// Hooks can be registered at the provider level (applied to all requests)
// and overridden per-request via ChatRequest.Metadata["hooks"] (*Hooks).
// Per-request hooks replace provider hooks field-by-field (nil fields
// fall back to the provider-level hook).
type Hooks struct {
	// BeforeRequest is called before each API call (once per ReAct round).
	// It receives a pointer to the request and can modify it.
	// Return an error to abort the entire stream.
	BeforeRequest func(ctx context.Context, req *ChatRequest) error

	// AfterResponse is called after a complete API response (each ReAct round).
	// stopReason is the model's stop_reason for this round.
	AfterResponse func(ctx context.Context, req *ChatRequest, stopReason string, usage Usage) error

	// OnEvent is called for every event emitted during streaming.
	// This is a side-effect hook — it cannot block or modify the event.
	OnEvent func(ctx context.Context, evt Event)

	// BeforeToolCall is called before executing a client-side tool.
	// It can modify the input JSON or skip execution entirely.
	// modifiedInput replaces the original input (if not skipped).
	BeforeToolCall func(ctx context.Context, name string, input json.RawMessage) (modifiedInput json.RawMessage, skip bool)

	// AfterToolCall is called after executing a client-side tool.
	AfterToolCall func(ctx context.Context, name string, input json.RawMessage, result string, isError bool)
}

// MergeHooks combines provider-level and request-level hooks.
// Request hooks take precedence. If requestHooks is nil, returns defaultHooks.
// If defaultHooks is nil, returns requestHooks.
// Otherwise returns a new Hooks where each non-nil field in requestHooks
// overrides the corresponding field in defaultHooks.
func MergeHooks(defaultHooks, requestHooks *Hooks) *Hooks {
	if requestHooks == nil {
		return defaultHooks
	}
	if defaultHooks == nil {
		return requestHooks
	}

	merged := &Hooks{}
	*merged = *defaultHooks // copy defaults

	if requestHooks.BeforeRequest != nil {
		merged.BeforeRequest = requestHooks.BeforeRequest
	}
	if requestHooks.AfterResponse != nil {
		merged.AfterResponse = requestHooks.AfterResponse
	}
	if requestHooks.OnEvent != nil {
		merged.OnEvent = requestHooks.OnEvent
	}
	if requestHooks.BeforeToolCall != nil {
		merged.BeforeToolCall = requestHooks.BeforeToolCall
	}
	if requestHooks.AfterToolCall != nil {
		merged.AfterToolCall = requestHooks.AfterToolCall
	}
	return merged
}

// ResolveHooks extracts hooks from a ChatRequest's Metadata and merges them
// with the provider-level defaults.
func ResolveHooks(defaultHooks *Hooks, req *ChatRequest) *Hooks {
	if req.Metadata == nil {
		return defaultHooks
	}
	rh, ok := req.Metadata["hooks"].(*Hooks)
	if !ok || rh == nil {
		return defaultHooks
	}
	return MergeHooks(defaultHooks, rh)
}
