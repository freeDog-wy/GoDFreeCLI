package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// eventMapper tracks the current state while mapping SDK events to our Event types.
// It is single-use per streaming response (not safe for concurrent use).
type eventMapper struct {
	// Accumulated state
	currentBlockType  llm.ContentBlockType
	currentBlockIndex int
	currentToolID     string
	currentToolName   string
	inputJSONBuf      []byte // accumulates InputJSONDelta fragments

	// Collected tool calls for this round
	toolCalls []anthropic.ToolUseBlock

	// Accumulated stop reason and usage
	stopReason string
	usage      llm.Usage
}

// newEventMapper creates a fresh mapper for one streaming response.
func newEventMapper() *eventMapper {
	return &eventMapper{}
}

// mapEvent converts an Anthropic SDK event union to zero or more llm.Event values.
// Some SDK events produce no output (accumulation only); others produce multiple.
func (m *eventMapper) mapEvent(event anthropic.MessageStreamEventUnion) []llm.Event {
	switch ev := event.AsAny().(type) {

	case anthropic.MessageStartEvent:
		return m.handleMessageStart(ev)

	case anthropic.ContentBlockStartEvent:
		return m.handleContentBlockStart(ev)

	case anthropic.ContentBlockDeltaEvent:
		return m.handleContentBlockDelta(ev)

	case anthropic.ContentBlockStopEvent:
		return m.handleContentBlockStop()

	case anthropic.MessageDeltaEvent:
		return m.handleMessageDelta(ev)

	case anthropic.MessageStopEvent:
		return m.handleMessageStop()
	}
	return nil
}

func (m *eventMapper) handleMessageStart(ev anthropic.MessageStartEvent) []llm.Event {
	return []llm.Event{
		llm.MessageStartEvent{Model: ev.Message.Model},
	}
}

func (m *eventMapper) handleContentBlockStart(ev anthropic.ContentBlockStartEvent) []llm.Event {
	m.currentBlockIndex = int(ev.Index)
	m.inputJSONBuf = nil // reset for new block

	switch block := ev.ContentBlock.AsAny().(type) {

	case anthropic.TextBlock:
		m.currentBlockType = llm.BlockTypeText
		m.currentToolID = ""
		m.currentToolName = ""

	case anthropic.ThinkingBlock:
		m.currentBlockType = llm.BlockTypeThinking
		m.currentToolID = ""
		m.currentToolName = ""

	case anthropic.ToolUseBlock:
		m.currentBlockType = llm.BlockTypeToolUse
		m.currentToolID = block.ID
		m.currentToolName = block.Name
		// Input comes via InputJSONDelta later

	case anthropic.ServerToolUseBlock:
		m.currentBlockType = llm.BlockTypeServerToolUse
		m.currentToolID = block.ID
		m.currentToolName = string(block.Name)

	case anthropic.WebSearchToolResultBlock:
		m.currentBlockType = llm.BlockTypeWebSearchResult
		m.currentToolID = ""
		m.currentToolName = ""

		// Emit a ServerToolResultEvent with web search items
		var items []llm.WebSearchItem
		for _, r := range block.Content.OfWebSearchResultBlockArray {
			items = append(items, llm.WebSearchItem{
				Title:   r.Title,
				URL:     r.URL,
				Snippet: r.PageAge, // best available context
			})
		}
		return []llm.Event{
			llm.ContentBlockStartEvent{
				Index:     int(ev.Index),
				BlockType: m.currentBlockType,
			},
			llm.ServerToolResultEvent{
				Index:          int(ev.Index),
				ToolType:       "web_search",
				WebSearchItems: items,
			},
		}

	case anthropic.WebFetchToolResultBlock:
		m.currentBlockType = llm.BlockTypeWebFetchResult
		m.currentToolID = ""
		m.currentToolName = ""

		content := fmt.Sprintf("URL: %s", block.Content.URL)
		return []llm.Event{
			llm.ContentBlockStartEvent{
				Index:     int(ev.Index),
				BlockType: m.currentBlockType,
			},
			llm.ServerToolResultEvent{
				Index:    int(ev.Index),
				ToolType: "web_fetch",
				Content:  content,
			},
		}

	case anthropic.CodeExecutionToolResultBlock:
		m.currentBlockType = llm.BlockTypeCodeExecResult
		m.currentToolID = ""
		m.currentToolName = ""

		output := block.Content.Stdout
		if output == "" {
			output = block.Content.EncryptedStdout
		}
		return []llm.Event{
			llm.ContentBlockStartEvent{
				Index:     int(ev.Index),
				BlockType: m.currentBlockType,
			},
			llm.ServerToolResultEvent{
				Index:    int(ev.Index),
				ToolType: "code_execution",
				Content:  output,
			},
		}

	default:
		m.currentBlockType = ""
		m.currentToolID = ""
		m.currentToolName = ""
	}

	evt := llm.ContentBlockStartEvent{
		Index:        int(ev.Index),
		BlockType:    m.currentBlockType,
		ToolCallID:   m.currentToolID,
		ToolCallName: m.currentToolName,
	}
	return []llm.Event{evt}
}

func (m *eventMapper) handleContentBlockDelta(ev anthropic.ContentBlockDeltaEvent) []llm.Event {
	switch delta := ev.Delta.AsAny().(type) {

	case anthropic.TextDelta:
		return []llm.Event{llm.TextDeltaEvent{Text: delta.Text}}

	case anthropic.InputJSONDelta:
		m.inputJSONBuf = append(m.inputJSONBuf, delta.PartialJSON...)
		// Accumulated silently; emitted on block stop

	case anthropic.ThinkingDelta:
		return []llm.Event{llm.ThinkingEvent{Thinking: delta.Thinking}}

	case anthropic.SignatureDelta:
		return []llm.Event{llm.ThinkingEvent{Signature: delta.Signature}}
	}
	return nil
}

func (m *eventMapper) handleContentBlockStop() []llm.Event {
	events := []llm.Event{
		llm.ContentBlockEndEvent{
			Index:     m.currentBlockIndex,
			BlockType: m.currentBlockType,
		},
	}

	// If this was a client tool_use block, emit the complete ToolCallEvent
	if m.currentBlockType == llm.BlockTypeToolUse && len(m.inputJSONBuf) > 0 {
		events = append(events, llm.ToolCallEvent{
			ID:    m.currentToolID,
			Name:  m.currentToolName,
			Input: json.RawMessage(m.inputJSONBuf),
		})
	}

	// If this was a client tool_use block, add to collection for the ReAct loop
	if m.currentBlockType == llm.BlockTypeToolUse {
		// Reconstruct via Accumulate? No — the Accumulate call in the main loop
		// already tracks this. But we duplicate for clarity in the ReAct loop.
	}

	return events
}

func (m *eventMapper) handleMessageDelta(ev anthropic.MessageDeltaEvent) []llm.Event {
	m.stopReason = string(ev.Delta.StopReason)
	m.usage = llm.Usage{
		InputTokens:  int64(ev.Usage.InputTokens),
		OutputTokens: int64(ev.Usage.OutputTokens),
	}
	// Accumulated silently; emitted with MessageStop
	return nil
}

func (m *eventMapper) handleMessageStop() []llm.Event {
	return []llm.Event{
		llm.MessageEndEvent{
			StopReason: m.stopReason,
			Usage:      m.usage,
		},
	}
}
