// Package mcp provides an MCP (Model Context Protocol) client that
// connects to external MCP servers and registers their tools into the
// unified tool registry.
//
// Usage:
//
//	mcpClient, err := mcp.Connect(ctx, mcp.ServerConfig{
//	    Command: "npx",
//	    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "."},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mcpClient.Close()
//
//	mcpClient.RegisterTo(ctx, reg)
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
	"github.com/freeDog-wy/GoDFreeCLI/internal/registry"
)

// ServerConfig describes how to launch an MCP server process.
type ServerConfig struct {
	// Command is the executable to run (e.g. "npx", "python", "uvx").
	Command string
	// Args are the arguments to the command.
	Args []string
}

// Client wraps an MCP SDK client session and manages its lifecycle.
// Use Connect to create one, then RegisterTo to add its tools to
// a registry.
type Client struct {
	cfg     ServerConfig
	client  *sdkmcp.Client
	session *sdkmcp.ClientSession
}

// Connect launches an MCP server process and performs the MCP handshake.
// The returned Client must be closed when no longer needed.
func Connect(ctx context.Context, cfg ServerConfig) (*Client, error) {
	c := &Client{cfg: cfg}

	mcpClient := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "GoDFreeCLI",
		Version: "1.0.0",
	}, nil)

	transport := &sdkmcp.CommandTransport{
		Command: exec.Command(cfg.Command, cfg.Args...),
	}

	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect to %s: %w", cfg.Command, err)
	}

	c.client = mcpClient
	c.session = session
	return c, nil
}

// Close terminates the MCP session and shuts down the server process.
func (c *Client) Close() error {
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// RegisterTo lists all tools from the MCP server and registers them
// into the given registry. Each MCP tool is wrapped in an executor that
// forwards calls to the MCP server.
func (c *Client) RegisterTo(ctx context.Context, reg *registry.Registry) error {
	tools, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list tools from %s: %w", c.cfg.Command, err)
	}

	for _, mcpTool := range tools.Tools {
		llmTool := convertTool(mcpTool)
		exec := c.makeExecutor(mcpTool.Name)

		if err := reg.Register(llmTool, exec); err != nil {
			return fmt.Errorf("register mcp tool %q: %w", mcpTool.Name, err)
		}
	}

	return nil
}

// makeExecutor returns an llm.ToolExecutor that forwards calls to
// the MCP server session.
func (c *Client) makeExecutor(name string) llm.ToolExecutor {
	return func(ctx context.Context, _ string, input json.RawMessage) (string, bool) {
		// Parse JSON arguments into a map for the MCP SDK
		var args map[string]any
		if len(input) > 0 {
			if err := json.Unmarshal(input, &args); err != nil {
				return fmt.Sprintf("invalid arguments for %s: %v", name, err), true
			}
		}

		result, err := c.session.CallTool(ctx, &sdkmcp.CallToolParams{
			Name:      name,
			Arguments: args,
		})
		if err != nil {
			return fmt.Sprintf("mcp call %s failed: %v", name, err), true
		}

		// Extract text from content blocks
		text := extractText(result)
		return text, result.IsError
	}
}

// ── conversion helpers ────────────────────────────────────

// convertTool converts an MCP tool definition to an llm.Tool.
func convertTool(t *sdkmcp.Tool) llm.Tool {
	return llm.Tool{
		Name:        t.Name,
		Description: coalesce(t.Description, t.Title),
		Parameters:  convertSchema(t.InputSchema),
	}
}

// convertSchema converts an MCP InputSchema (any → usually map[string]any)
// to an llm.ToolSchema. Returns nil if the schema is empty or unparseable.
func convertSchema(schema any) *llm.ToolSchema {
	if schema == nil {
		return nil
	}

	m, ok := schema.(map[string]any)
	if !ok {
		return nil
	}

	out := &llm.ToolSchema{}

	if typ, ok := m["type"].(string); ok {
		out.Type = typ
	}

	if props, ok := m["properties"].(map[string]any); ok {
		out.Properties = make(map[string]llm.Property, len(props))
		for k, v := range props {
			propMap, ok := v.(map[string]any)
			if !ok {
				continue
			}
			prop := llm.Property{}
			if t, ok := propMap["type"].(string); ok {
				prop.Type = t
			}
			if desc, ok := propMap["description"].(string); ok {
				prop.Description = desc
			}
			if enum, ok := propMap["enum"].([]any); ok {
				for _, e := range enum {
					if s, ok := e.(string); ok {
						prop.Enum = append(prop.Enum, s)
					}
				}
			}
			out.Properties[k] = prop
		}
	}

	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				out.Required = append(out.Required, s)
			}
		}
	}

	return out
}

// extractText pulls text content from an MCP CallToolResult.
// Multiple text blocks are joined with newlines.
func extractText(result *sdkmcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var texts []string
	for _, block := range result.Content {
		if tc, ok := block.(*sdkmcp.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}
	if len(texts) == 0 {
		return "(no text output)"
	}
	if len(texts) == 1 {
		return texts[0]
	}
	// Join multiple text blocks
	out := ""
	for i, t := range texts {
		if i > 0 {
			out += "\n"
		}
		out += t
	}
	return out
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
