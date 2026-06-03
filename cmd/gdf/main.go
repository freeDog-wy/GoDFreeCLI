package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/freeDog-wy/GoDFreeCLI/internal/agent"
	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
	"github.com/freeDog-wy/GoDFreeCLI/internal/llm/anthropic"
	"github.com/freeDog-wy/GoDFreeCLI/internal/mcp"
	"github.com/freeDog-wy/GoDFreeCLI/internal/registry"
	"github.com/freeDog-wy/GoDFreeCLI/internal/skill"
	skillbuiltin "github.com/freeDog-wy/GoDFreeCLI/internal/skill/builtin"
	"github.com/freeDog-wy/GoDFreeCLI/internal/tool"
)

var verbose bool

func main() {
	for _, a := range os.Args[1:] {
		if a == "--verbose" || a == "-v" {
			verbose = true
		}
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if apiKey == "" {
		apiKey = "sk-placeholder"
	}
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	reg := registry.New()

	toolSet := tool.DefaultToolSet()
	toolSet.Browser = &tool.BrowserExecConfig{Headless: false}

	builtinTools := tool.AllTools(toolSet)
	builtinExec, browserMgr := tool.NewExecutorRegistry(tool.ExecConfig{
		ProjectRoot: ".",
		ToolSet:     toolSet,
	})
	if browserMgr != nil {
		defer browserMgr.Shutdown()
		builtinTools = append(builtinTools, tool.BrowserTools()...)
	}
	if err := reg.RegisterAll(builtinTools, builtinExec); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register built-in tools: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mcpServers := loadMCPServers()
	for _, srv := range mcpServers {
		mcpClient, err := mcp.Connect(ctx, srv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] MCP server %s: %v - skipping\n", srv.Command, err)
			continue
		}
		if err := mcpClient.RegisterTo(ctx, reg); err != nil {
			fmt.Fprintf(os.Stderr, "[warn] MCP register %s: %v - skipping\n", srv.Command, err)
			mcpClient.Close()
			continue
		}
		defer mcpClient.Close()
		if verbose {
			fmt.Printf("[mcp] Connected to %s %v\n", srv.Command, srv.Args)
		}
	}

	skillReg := skill.NewRegistry(reg)
	skillbuiltin.LoadSkills(skillReg, skillbuiltin.DefaultSkillSet())
	fsLoaded, err := skill.LoadFromDir(skillReg, "skills")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[warn] Failed to load skills from skills/: %v\n", err)
	}

	skillTools := skill.SkillTools()
	skillExecs := skill.MakeSkillExecutors(skillReg)
	if err := reg.RegisterAll(skillTools, skillExecs); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] Failed to register skill tools: %v\n", err)
	}

	if verbose {
		fmt.Printf("[registry] %d tools registered\n", len(reg.Tools()))
		fmt.Printf("[skills] %d loaded (%d from filesystem)\n", len(skillReg.List()), fsLoaded)
	}

	var hooks *llm.Hooks
	if verbose {
		hooks = &llm.Hooks{
			BeforeRequest: func(ctx context.Context, req *llm.ChatRequest) error {
				fmt.Printf("[hook] BeforeRequest: model=%s, messages=%d\n", req.Model, len(req.Messages))
				return nil
			},
			AfterResponse: func(ctx context.Context, req *llm.ChatRequest, stopReason string, usage llm.Usage) error {
				fmt.Printf("[hook] AfterResponse: stop_reason=%s, in=%d, out=%d\n", stopReason, usage.InputTokens, usage.OutputTokens)
				return nil
			},
			BeforeToolCall: func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, bool) {
				fmt.Printf("[hook] BeforeToolCall: name=%s input=%s\n", name, string(input))
				return input, false
			},
			AfterToolCall: func(ctx context.Context, name string, input json.RawMessage, result string, isError bool) {
				fmt.Printf("[hook] AfterToolCall: name=%s result=%s (error=%v)\n", name, result, isError)
			},
		}
	}

	provider := anthropic.New(
		anthropic.WithAPIKey(apiKey),
		anthropic.WithBaseURL(baseURL),
		anthropic.WithHooks(hooks),
		anthropic.WithToolExecutor(reg.Execute),
	)

	messages := []llm.Message{}
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("GoDFreeCLI - type /exit to quit")
	fmt.Println(strings.Repeat("-", 40))

	for {
		fmt.Print("\nYou: ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/exit" || input == "/quit" {
			fmt.Println("Goodbye!")
			break
		}

		messages = append(messages, llm.NewUserMessage(input))

		var err error
		messages, err = runTurn(ctx, provider, reg, skillReg, messages, scanner)
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		}
	}
}

// ── turn state ────────────────────────────────────────────

type turnRound struct {
	assistant   []llm.ContentBlock
	toolResults []llm.ContentBlock
}

type turnResult struct {
	rounds        []turnRound
	askUsers      []*llm.AskUserEvent
	useSkills     []*llm.UseSkillEvent
	delegateTasks []*llm.DelegateTaskEvent
	hasError      bool
	done          bool
}

func (r *turnResult) curRound() *turnRound {
	if len(r.rounds) == 0 {
		r.rounds = append(r.rounds, turnRound{})
	}
	return &r.rounds[len(r.rounds)-1]
}

// ── runTurn ───────────────────────────────────────────────

func runTurn(
	ctx context.Context,
	provider llm.Provider,
	reg *registry.Registry,
	skillReg *skill.Registry,
	messages []llm.Message,
	scanner *bufio.Scanner,
) ([]llm.Message, error) {

	origLen := len(messages) - 1

	for {
		system := skillReg.ListPrompt()
		if active := skillReg.ActivePrompt(); active != "" {
			system += "\n\n## Active Skills\n\n" + active
		}

		req := &llm.ChatRequest{
			Model:     "claude-sonnet-4-6",
			MaxTokens: 4096,
			System:    system,
			Messages:  messages,
			Tools:     reg.Tools(),
		}

		result := &turnResult{}

		fmt.Print("\nClaude: ")

		for event := range provider.StreamChat(ctx, req) {
			handleEvent(event, result)
		}

		if result.hasError {
			return messages[:origLen], fmt.Errorf("stream error")
		}

		// Rebuild multi-round history
		for _, round := range result.rounds {
			if len(round.assistant) > 0 {
				messages = append(messages, llm.NewAssistantMessage(round.assistant...))
			}
			if len(round.toolResults) > 0 {
				messages = append(messages, llm.Message{
					Role:    llm.RoleUser,
					Content: round.toolResults,
				})
			}
		}

		// Handle ask_user
		if len(result.askUsers) > 0 {
			var toolResults []llm.ContentBlock
			for _, ask := range result.askUsers {
				var input tool.AskUserInput
				json.Unmarshal(ask.Input, &input)

				fmt.Printf("\n[Question] %s\n", input.Question)
				fmt.Print("Your answer: ")
				if !scanner.Scan() {
					return messages, nil
				}
				answer := strings.TrimSpace(scanner.Text())

				toolResults = append(toolResults, llm.ContentBlock{
					Type:       llm.BlockTypeToolResult,
					ToolCallID: ask.ID,
					ToolResult: answer,
				})
			}

			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: toolResults,
			})

			continue
		}

		// Handle use_skill
		if len(result.useSkills) > 0 {
			for _, us := range result.useSkills {
				var input skill.UseSkillInput
				json.Unmarshal(us.Input, &input)

				prompt, err := skillReg.Activate(input.Name)
				if err != nil {
					fmt.Fprintf(os.Stderr, "\n[warn] %v\n", err)
				}
				if verbose && prompt != "" {
					fmt.Printf("\n[skill] Activated %s (%d chars prompt)\n",
						input.Name, len(prompt))
				}
			}
			continue
		}

		// Handle delegate_task -- spawn sub-agents for parallel work
		if len(result.delegateTasks) > 0 {
			for _, dt := range result.delegateTasks {
				var input tool.DelegateTaskInput
				json.Unmarshal(dt.Input, &input)

				toolFilter := parseToolFilter(input.Tools)
				if verbose {
					fmt.Printf("\n[delegate] delegating: %s\n", input.Description)
				}

				subResult := agent.Run(ctx, provider, agent.Config{
					Description: input.Description,
					AllTools:    reg.Tools(),
					ToolFilter:  toolFilter,
					MaxRounds:   input.MaxRounds,
				})

				msg := agent.SubmitResult(subResult)
				messages = append(messages, llm.Message{
					Role: llm.RoleUser,
					Content: []llm.ContentBlock{
						{Type: llm.BlockTypeToolResult, ToolCallID: dt.ID, ToolResult: msg},
					},
				})
			}
			continue
		}

		// Normal completion
		fmt.Println()
		return messages, nil
	}
}

// ── handleEvent ───────────────────────────────────────────

func handleEvent(event llm.Event, r *turnResult) {
	switch ev := event.(type) {

	case llm.MessageStartEvent:
		r.rounds = append(r.rounds, turnRound{})
		if verbose {
			fmt.Printf("\n[message_start] model=%s\n", ev.Model)
		}

	case llm.TextDeltaEvent:
		fmt.Print(ev.Text)
		cr := r.curRound()
		if len(cr.assistant) > 0 && cr.assistant[len(cr.assistant)-1].Type == llm.BlockTypeText {
			cr.assistant[len(cr.assistant)-1].Text += ev.Text
		} else {
			cr.assistant = append(cr.assistant, llm.ContentBlock{
				Type: llm.BlockTypeText,
				Text: ev.Text,
			})
		}

	case llm.ToolCallEvent:
		if verbose {
			fmt.Printf("\n  [tool_call] id=%s name=%s input=%s\n",
				ev.ID, ev.Name, string(ev.Input))
		}
		cr := r.curRound()
		cr.assistant = append(cr.assistant, llm.ContentBlock{
			Type:       llm.BlockTypeToolUse,
			ToolCallID: ev.ID,
			ToolName:   ev.Name,
			ToolInput:  ev.Input,
		})

	case llm.ToolResultEvent:
		if verbose {
			fmt.Printf("  [tool_result] id=%s name=%s result=%s (error=%v)\n",
				ev.ID, ev.Name, ev.Result, ev.IsError)
		}
		cr := r.curRound()
		cr.toolResults = append(cr.toolResults, llm.ContentBlock{
			Type:       llm.BlockTypeToolResult,
			ToolCallID: ev.ID,
			ToolName:   ev.Name,
			ToolResult: ev.Result,
			IsError:    ev.IsError,
		})

	case llm.AskUserEvent:
		if verbose {
			fmt.Printf("\n  [ask_user] id=%s name=%s input=%s\n",
				ev.ID, ev.Name, string(ev.Input))
		}
		r.askUsers = append(r.askUsers, &ev)

	case llm.UseSkillEvent:
		if verbose {
			fmt.Printf("\n  [use_skill] id=%s input=%s\n", ev.ID, string(ev.Input))
		}
		r.useSkills = append(r.useSkills, &ev)

	case llm.DelegateTaskEvent:
		if verbose {
			fmt.Printf("\n  [delegate_task] id=%s input=%s\n", ev.ID, string(ev.Input))
		}
		r.delegateTasks = append(r.delegateTasks, &ev)

	case llm.ThinkingEvent:
		if verbose && ev.Thinking != "" {
			fmt.Printf("%s", ev.Thinking)
		}

	case llm.MessageEndEvent:
		if verbose {
			fmt.Printf("[message_end] stop_reason=%s\n", ev.StopReason)
		}

	case llm.ErrorEvent:
		fmt.Printf("\nError: %v\n", ev.Err)
		r.hasError = true

	case llm.DoneEvent:
		r.done = true
	}
}

// ── helpers ───────────────────────────────────────────────

// parseToolFilter converts a comma-separated tool name list to a set.
func parseToolFilter(raw string) map[string]bool {
	if raw == "" {
		return nil // nil means "all tools allowed"
	}
	set := make(map[string]bool)
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			set[name] = true
		}
	}
	return set
}

// ── MCP server configuration ──────────────────────────────

func loadMCPServers() []mcp.ServerConfig {
	raw := os.Getenv("MCP_SERVERS")
	if raw == "" {
		return nil
	}

	var servers []struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal([]byte(raw), &servers); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] invalid MCP_SERVERS JSON: %v\n", err)
		return nil
	}

	out := make([]mcp.ServerConfig, len(servers))
	for i, s := range servers {
		out[i] = mcp.ServerConfig{Command: s.Command, Args: s.Args}
	}
	return out
}
