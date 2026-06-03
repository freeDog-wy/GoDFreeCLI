package anthropic

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// StreamChat implements [llm.Provider].
//
// It converts the request to Anthropic SDK types, runs the ReAct tool-calling
// loop internally, and emits provider-agnostic events through the returned
// channel.  The channel is closed after [llm.DoneEvent] (success) or
// [llm.ErrorEvent] (failure).
func (p *Provider) StreamChat(ctx context.Context, req *llm.ChatRequest) <-chan llm.Event {
	ch := make(chan llm.Event, 64)

	go func() {
		defer close(ch)
		p.streamLoop(ctx, req, ch)
	}()

	return ch
}

// ── main loop ─────────────────────────────────────────────

func (p *Provider) streamLoop(ctx context.Context, req *llm.ChatRequest, ch chan<- llm.Event) {
	hooks := llm.ResolveHooks(p.defaultHooks, req)
	executor := resolveExecutor(p.toolExecutor, req)

	maxRounds := req.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}

	// Convert initial messages and tools
	messages := convertMessages(req.Messages)
	tools := convertTools(req.Tools)

	for round := 0; round < maxRounds; round++ {
		// Check context before each round
		if ctx.Err() != nil {
			ch <- llm.ErrorEvent{Err: ctx.Err()}
			return
		}

		// ── BeforeRequest hook ──
		if hooks != nil && hooks.BeforeRequest != nil {
			if err := hooks.BeforeRequest(ctx, req); err != nil {
				ch <- llm.ErrorEvent{Err: fmt.Errorf("before_request hook: %w", err)}
				return
			}
		}

		// ── Build SDK request ──
		sdkReq := anthropic.MessageNewParams{
			Model:     anthropic.Model(req.Model),
			MaxTokens: int64(req.MaxTokens),
			Messages:  messages,
			Tools:     tools,
		}
		if req.System != "" {
			sdkReq.System = []anthropic.TextBlockParam{{Text: req.System}}
		}
		if req.Temperature != nil {
			sdkReq.Temperature = param.Opt[float64]{Value: *req.Temperature}
		}

		// ── Stream ──
		stream := p.client.Messages.NewStreaming(ctx, sdkReq)
		msg := anthropic.Message{}
		mapper := newEventMapper()

		for stream.Next() {
			event := stream.Current()

			// Accumulate into a full message (for history + tool extraction)
			if err := msg.Accumulate(event); err != nil {
				ch <- llm.ErrorEvent{Err: fmt.Errorf("accumulate: %w", err)}
				return
			}

			// Map to our event types and emit
			for _, evt := range mapper.mapEvent(event) {
				// OnEvent hook
				if hooks != nil && hooks.OnEvent != nil {
					hooks.OnEvent(ctx, evt)
				}
				ch <- evt
			}
		}

		if stream.Err() != nil {
			ch <- llm.ErrorEvent{Err: fmt.Errorf("stream: %w", stream.Err())}
			return
		}

		// ── AfterResponse hook ──
		if hooks != nil && hooks.AfterResponse != nil {
			if err := hooks.AfterResponse(ctx, req, string(msg.StopReason), llm.Usage{
				InputTokens:  int64(msg.Usage.InputTokens),
				OutputTokens: int64(msg.Usage.OutputTokens),
			}); err != nil {
				ch <- llm.ErrorEvent{Err: fmt.Errorf("after_response hook: %w", err)}
				return
			}
		}

		// ── Collect client tool calls ──
		var toolCalls []anthropic.ToolUseBlock
		for _, block := range msg.Content {
			if b, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				toolCalls = append(toolCalls, b)
			}
		}

		// ── Append assistant message to history ──
		messages = append(messages, msg.ToParam())

		// ── Check stop reason ──
		stopReason := string(msg.StopReason)
		switch stopReason {
		case "end_turn", "max_tokens", "stop_sequence", "refusal":
			ch <- llm.DoneEvent{}
			return
		case "tool_use":
			if len(toolCalls) == 0 {
				ch <- llm.DoneEvent{}
				return
			}
			// Continue to execute tools
		default:
			if len(toolCalls) == 0 {
				ch <- llm.DoneEvent{}
				return
			}
		}

		// ── Intercept special tools ──
		// ask_user and use_skill are intercepted before execution.
		// They pause the ReAct loop and let the consumer handle
		// them (ask user / activate skill).
		var (
			regularCalls      []anthropic.ToolUseBlock
			askUserCalls      []anthropic.ToolUseBlock
			useSkillCalls     []anthropic.ToolUseBlock
			delegateTaskCalls []anthropic.ToolUseBlock
		)
		for _, tc := range toolCalls {
			switch tc.Name {
			case "ask_user":
				askUserCalls = append(askUserCalls, tc)
			case "use_skill":
			case "delegate_task":
				delegateTaskCalls = append(delegateTaskCalls, tc)
				useSkillCalls = append(useSkillCalls, tc)
			default:
				regularCalls = append(regularCalls, tc)
			}
		}

		if len(askUserCalls) > 0 {
			for _, tc := range askUserCalls {
				ch <- llm.AskUserEvent{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: tc.Input,
				}
			}
			ch <- llm.DoneEvent{}
			return
		}

		if len(delegateTaskCalls) > 0 {
			for _, tc := range delegateTaskCalls {
				ch <- llm.DelegateTaskEvent{
					ID:    tc.ID,
					Input: tc.Input,
				}
			}
			ch <- llm.DoneEvent{}
			return
		}

		if len(useSkillCalls) > 0 {
			for _, tc := range useSkillCalls {
				ch <- llm.UseSkillEvent{
					ID:    tc.ID,
					Input: tc.Input,
				}
			}
			ch <- llm.DoneEvent{}
			return
		}

		// Execute remaining (regular) tools
		toolCalls = regularCalls

		// ── Execute client tools ──
		if executor == nil {
			ch <- llm.DoneEvent{}
			return
		}

		var toolResults []anthropic.ContentBlockParamUnion
		for _, tc := range toolCalls {
			input := tc.Input

			// BeforeToolCall hook
			if hooks != nil && hooks.BeforeToolCall != nil {
				modified, skip := hooks.BeforeToolCall(ctx, tc.Name, input)
				if skip {
					continue
				}
				if modified != nil {
					input = modified
				}
			}

			result, isError := executor(ctx, tc.Name, input)

			// AfterToolCall hook
			if hooks != nil && hooks.AfterToolCall != nil {
				hooks.AfterToolCall(ctx, tc.Name, input, result, isError)
			}

			ch <- llm.ToolResultEvent{
				ID:      tc.ID,
				Name:    tc.Name,
				Result:  result,
				IsError: isError,
			}

			toolResults = append(toolResults,
				anthropic.NewToolResultBlock(tc.ID, result, isError),
			)
		}

		// Append tool results as a user message
		if len(toolResults) > 0 {
			messages = append(messages, anthropic.NewUserMessage(toolResults...))
		}
	}

	// Max rounds exceeded
	ch <- llm.DoneEvent{}
}

// ── type conversion helpers ───────────────────────────────

// convertMessages converts llm messages to Anthropic SDK message params.
func convertMessages(msgs []llm.Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, convertMessage(m))
	}
	return out
}

func convertMessage(msg llm.Message) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion
	for _, b := range msg.Content {
		blocks = append(blocks, convertContentBlock(b))
	}

	switch msg.Role {
	case llm.RoleAssistant:
		return anthropic.NewAssistantMessage(blocks...)
	default:
		return anthropic.NewUserMessage(blocks...)
	}
}

func convertContentBlock(block llm.ContentBlock) anthropic.ContentBlockParamUnion {
	switch block.Type {
	case llm.BlockTypeText:
		return anthropic.NewTextBlock(block.Text)

	case llm.BlockTypeToolUse:
		return anthropic.NewToolUseBlock(block.ToolCallID, block.ToolInput, block.ToolName)

	case llm.BlockTypeToolResult:
		return anthropic.NewToolResultBlock(block.ToolCallID, block.ToolResult, block.IsError)

	default:
		// Fallback: treat as text
		if block.Text != "" {
			return anthropic.NewTextBlock(block.Text)
		}
		return anthropic.NewTextBlock(string(block.Data))
	}
}

// convertTools converts llm tools to Anthropic SDK tool params.
func convertTools(tools []llm.Tool) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		out = append(out, convertTool(t))
	}
	return out
}

func convertTool(tool llm.Tool) anthropic.ToolUnionParam {
	schema := anthropic.ToolInputSchemaParam{}
	if tool.Parameters != nil {
		props := make(map[string]any, len(tool.Parameters.Properties))
		for k, v := range tool.Parameters.Properties {
			prop := map[string]any{
				"type": v.Type,
			}
			if v.Description != "" {
				prop["description"] = v.Description
			}
			if len(v.Enum) > 0 {
				prop["enum"] = v.Enum
			}
			props[k] = prop
		}
		schema.Properties = props
		schema.Required = tool.Parameters.Required
	}

	desc := param.Opt[string]{}
	if tool.Description != "" {
		desc = param.Opt[string]{Value: tool.Description}
	}

	return anthropic.ToolUnionParam{
		OfTool: &anthropic.ToolParam{
			Name:        tool.Name,
			Description: desc,
			InputSchema: schema,
		},
	}
}

// ── helpers ───────────────────────────────────────────────

// resolveExecutor returns the per-request executor if available,
// otherwise the provider default.
func resolveExecutor(defaultExec llm.ToolExecutor, req *llm.ChatRequest) llm.ToolExecutor {
	if req.Metadata == nil {
		return defaultExec
	}
	if exec, ok := req.Metadata["tool_executor"].(llm.ToolExecutor); ok && exec != nil {
		return exec
	}
	return defaultExec
}
