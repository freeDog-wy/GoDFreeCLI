// Package anthropic implements the llm.Provider interface using the Anthropic SDK.
//
// It encapsulates the full ReAct tool-calling loop internally and emits
// provider-agnostic llm.Event values through a channel.
package anthropic

import (
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// Provider implements llm.Provider using the Anthropic Messages API.
type Provider struct {
	client       anthropic.Client
	defaultHooks *llm.Hooks
	toolExecutor llm.ToolExecutor // fallback executor
}

// ── Option types ──────────────────────────────────────────

type config struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	hooks        *llm.Hooks
	toolExecutor llm.ToolExecutor
}

// Option configures the Anthropic provider.
type Option func(*config)

// WithBaseURL sets a custom API base URL (for proxies or compatible providers).
// Default: https://api.anthropic.com
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = url }
}

// WithAPIKey sets the API key.
func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *config) { c.httpClient = client }
}

// WithHooks sets the default lifecycle hooks for all requests.
// Per-request hooks (from ChatRequest.Metadata) override these field-by-field.
func WithHooks(hooks *llm.Hooks) Option {
	return func(c *config) { c.hooks = hooks }
}

// WithToolExecutor sets the default tool executor for all requests.
// Per-request executors (from ChatRequest.Metadata) override this.
func WithToolExecutor(exec llm.ToolExecutor) Option {
	return func(c *config) { c.toolExecutor = exec }
}

// ── Constructor ───────────────────────────────────────────

// New creates a new Anthropic provider.
//
// At minimum, WithAPIKey must be provided (or the ANTHROPIC_API_KEY env var set).
// Use WithBaseURL to point at a compatible provider (e.g., DeepSeek).
func New(opts ...Option) *Provider {
	cfg := &config{
		baseURL: "https://api.anthropic.com",
	}
	for _, o := range opts {
		o(cfg)
	}

	sdkOpts := []option.RequestOption{
		option.WithBaseURL(cfg.baseURL),
	}
	if cfg.apiKey != "" {
		sdkOpts = append(sdkOpts, option.WithAPIKey(cfg.apiKey))
	}
	if cfg.httpClient != nil {
		sdkOpts = append(sdkOpts, option.WithHTTPClient(cfg.httpClient))
	}

	return &Provider{
		client:       anthropic.NewClient(sdkOpts...),
		defaultHooks: cfg.hooks,
		toolExecutor: cfg.toolExecutor,
	}
}

// compile-time interface check
var _ llm.Provider = (*Provider)(nil)
