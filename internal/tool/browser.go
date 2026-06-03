package tool

import "github.com/freeDog-wy/GoDFreeCLI/internal/llm"

// ── Browser tool names ──

const (
	BrowserNavigate   = "browser_navigate"
	BrowserScreenshot = "browser_screenshot"
	BrowserGetContent = "browser_get_content"
	BrowserExecuteJS  = "browser_execute_js"
	BrowserClick      = "browser_click"
	BrowserTypeText   = "browser_type_text"
	BrowserPageInfo   = "browser_get_page_info"
	BrowserWaitFor    = "browser_wait_for"
	BrowserScroll     = "browser_scroll"
	BrowserManageTab  = "browser_manage_tab"
)

// ── browser_navigate ──────────────────────────────────────

func BrowserNavigateTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserNavigate,
		Description: "Navigate the browser to a URL. Returns the page title and URL to confirm success.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"url":        {Type: "string", Description: "The URL to navigate to"},
				"tab_id":     {Type: "string", Description: "Target tab ID. Uses default tab if omitted."},
				"timeout_ms": {Type: "integer", Description: "Timeout in milliseconds. Default: 30000."},
			},
			Required: []string{"url"},
		},
	}
}

type BrowserNavigateInput struct {
	URL       string `json:"url"`
	TabID     string `json:"tab_id,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// ── browser_screenshot ────────────────────────────────────

func BrowserScreenshotTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserScreenshot,
		Description: "Take a screenshot of the current page. Can capture full page, viewport, or a specific element. Returns the file path of the saved PNG.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"selector":    {Type: "string", Description: "CSS selector to capture a specific element. Captures the whole page if omitted."},
				"full_page":   {Type: "boolean", Description: "Capture full page (default: true)."},
				"quality":     {Type: "integer", Description: "Image quality 1-100. Default: 90."},
				"tab_id":      {Type: "string", Description: "Target tab ID."},
				"output_path": {Type: "string", Description: "Output file path. Auto-generated in temp dir if omitted."},
			},
		},
	}
}

type BrowserScreenshotInput struct {
	Selector   string `json:"selector,omitempty"`
	FullPage   *bool  `json:"full_page,omitempty"`
	Quality    int    `json:"quality,omitempty"`
	TabID      string `json:"tab_id,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
}

// ── browser_get_content ───────────────────────────────────

func BrowserGetContentTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserGetContent,
		Description: "Get the text or HTML content of a page element. Use to extract page data. Defaults to body innerText. Large outputs are truncated to prevent token overflow.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"selector": {Type: "string", Description: "CSS selector. Default: 'body'."},
				"format": {
					Type:        "string",
					Description: "Output format: 'text' (innerText), 'html' (innerHTML), or 'outer' (outerHTML). Default: 'text'.",
					Enum:        []string{"text", "html", "outer"},
				},
				"tab_id": {Type: "string", Description: "Target tab ID."},
			},
		},
	}
}

type BrowserGetContentInput struct {
	Selector string `json:"selector,omitempty"`
	Format   string `json:"format,omitempty"`
	TabID    string `json:"tab_id,omitempty"`
}

// ── browser_execute_js ────────────────────────────────────

func BrowserExecuteJSTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserExecuteJS,
		Description: "Execute JavaScript in the current page and return the result. Use for data extraction, DOM manipulation, or triggering behaviors. The return value is converted to a string.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"script": {Type: "string", Description: "JavaScript code to execute."},
				"tab_id": {Type: "string", Description: "Target tab ID."},
			},
			Required: []string{"script"},
		},
	}
}

type BrowserExecuteJSInput struct {
	Script string `json:"script"`
	TabID  string `json:"tab_id,omitempty"`
}

// ── browser_click ─────────────────────────────────────────

func BrowserClickTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserClick,
		Description: "Click an element matching a CSS selector. Automatically waits for the element to become visible before clicking.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"selector": {Type: "string", Description: "CSS selector of the element to click."},
				"tab_id":   {Type: "string", Description: "Target tab ID."},
			},
			Required: []string{"selector"},
		},
	}
}

type BrowserClickInput struct {
	Selector string `json:"selector"`
	TabID    string `json:"tab_id,omitempty"`
}

// ── browser_type_text ─────────────────────────────────────

func BrowserTypeTextTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserTypeText,
		Description: "Type text into an input field. Optionally clear existing content first, or submit the form after typing.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"selector": {Type: "string", Description: "CSS selector of the input field."},
				"text":     {Type: "string", Description: "Text to type."},
				"clear":    {Type: "boolean", Description: "Clear existing content before typing. Default: false."},
				"submit":   {Type: "boolean", Description: "Submit the form after typing. Default: false."},
				"tab_id":   {Type: "string", Description: "Target tab ID."},
			},
			Required: []string{"selector", "text"},
		},
	}
}

type BrowserTypeTextInput struct {
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Clear    bool   `json:"clear,omitempty"`
	Submit   bool   `json:"submit,omitempty"`
	TabID    string `json:"tab_id,omitempty"`
}

// ── browser_get_page_info ─────────────────────────────────

func BrowserPageInfoTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserPageInfo,
		Description: "Get basic page info: title, URL, and load state. Lightweight operation for checking page status.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"tab_id": {Type: "string", Description: "Target tab ID."},
			},
		},
	}
}

type BrowserPageInfoInput struct {
	TabID string `json:"tab_id,omitempty"`
}

// ── browser_wait_for ──────────────────────────────────────

func BrowserWaitForTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserWaitFor,
		Description: "Wait for a CSS selector to become ready in the page. Use after navigation or clicks to wait for dynamic content to load.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"selector":   {Type: "string", Description: "CSS selector to wait for."},
				"timeout_ms": {Type: "integer", Description: "Timeout in milliseconds. Default: 10000."},
				"tab_id":     {Type: "string", Description: "Target tab ID."},
			},
			Required: []string{"selector"},
		},
	}
}

type BrowserWaitForInput struct {
	Selector  string `json:"selector"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
	TabID     string `json:"tab_id,omitempty"`
}

// ── browser_scroll ────────────────────────────────────────

func BrowserScrollTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserScroll,
		Description: "Scroll the page. Positive values scroll down, negative scroll up. Use to trigger lazy loading or view different page regions.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"amount": {Type: "integer", Description: "Pixels to scroll. Positive = down, negative = up. Example: 500 scrolls down, -300 scrolls up."},
				"tab_id": {Type: "string", Description: "Target tab ID."},
			},
			Required: []string{"amount"},
		},
	}
}

type BrowserScrollInput struct {
	Amount int    `json:"amount"`
	TabID  string `json:"tab_id,omitempty"`
}

// ── browser_manage_tab ────────────────────────────────────

func BrowserManageTabTool() llm.Tool {
	return llm.Tool{
		Name:        BrowserManageTab,
		Description: "Manage browser tabs: 'new' creates a new tab (with optional URL), 'close' closes a tab, 'list' lists all tabs with page info.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"action": {
					Type:        "string",
					Description: "Action: 'new', 'close', or 'list'.",
					Enum:        []string{"new", "close", "list"},
				},
				"url":    {Type: "string", Description: "Initial URL when action='new'."},
				"tab_id": {Type: "string", Description: "Target tab ID (required for action='close')."},
			},
			Required: []string{"action"},
		},
	}
}

type BrowserManageTabInput struct {
	Action string `json:"action"`
	URL    string `json:"url,omitempty"`
	TabID  string `json:"tab_id,omitempty"`
}

// ── All browser tools helper ──────────────────────────────

// BrowserTools returns all browser automation tools.
func BrowserTools() []llm.Tool {
	return []llm.Tool{
		BrowserNavigateTool(),
		BrowserScreenshotTool(),
		BrowserGetContentTool(),
		BrowserExecuteJSTool(),
		BrowserClickTool(),
		BrowserTypeTextTool(),
		BrowserPageInfoTool(),
		BrowserWaitForTool(),
		BrowserScrollTool(),
		BrowserManageTabTool(),
	}
}
