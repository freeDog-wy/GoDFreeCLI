package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// ── ToolSet ────────────────────────────────────────────────

// ToolSet controls which categories of built-in tools are exposed
// to the model. Set categories to false to disable them entirely.
type ToolSet struct {
	// FileOps enables read_file, write_file, list_files, search_file,
	// and search_content tools.
	FileOps bool

	// Bash enables the execute_bash tool for running shell commands.
	Bash bool

	// Browser enables Chrome automation tools. When nil, browser
	// tools are disabled. Set to a non-nil BrowserExecConfig to
	// enable (requires Chrome installed).
	Browser *BrowserExecConfig

	// UserInput enables the ask_user tool for interactive prompts.
	UserInput bool
}

// DefaultToolSet returns a ToolSet with all categories enabled
// except browser (which requires Chrome).
func DefaultToolSet() ToolSet {
	return ToolSet{
		FileOps:   true,
		Bash:      true,
		UserInput: true,
	}
}

// BrowserExecConfig configures the browser automation subsystem.
type BrowserExecConfig struct {
	Headless      bool
	ProfileDir    string
	ScreenshotDir string
}

// ── ExecConfig ─────────────────────────────────────────────

// ExecConfig holds configuration for built-in tool executors.
type ExecConfig struct {
	// ProjectRoot is the base directory for resolving relative paths.
	ProjectRoot string

	// ToolSet controls which tool categories are enabled.
	ToolSet ToolSet
}

// NewExecutorRegistry returns a map of tool name → llm.ToolExecutor
// for built-in tools, respecting the categories enabled in cfg.ToolSet.
//
// The returned *BrowserManager (non-nil when browser is active) must
// be shut down via bm.Shutdown() when no longer needed.
func NewExecutorRegistry(cfg ExecConfig) (execMap map[string]llm.ToolExecutor, browserMgr *BrowserManager) {
	root := cfg.ProjectRoot
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	ts := cfg.ToolSet
	m := make(map[string]llm.ToolExecutor)

	// File operations
	if ts.FileOps {
		m[ReadFileName] = makeReadFile(absRoot)
		m[WriteFileName] = makeWriteFile(absRoot)
		m[ListFileName] = makeListFiles(absRoot)
		m[SearchFileName] = makeSearchFile(absRoot)
		m[GrepFileName] = makeGrep(absRoot)
	}

	// Bash
	if ts.Bash {
		m[BashName] = makeBash(absRoot)
	}

	// User input (ask_user is intercepted by the provider)
	if ts.UserInput {
		m[AskUserName] = makeAskUser()
	}

	// Delegate task (intercepted by the provider)
	m[DelegateTaskName] = makeNoop(DelegateTaskName)

	// Browser
	if ts.Browser != nil {
		screenshotDir := ts.Browser.ScreenshotDir
		if screenshotDir == "" {
			screenshotDir = filepath.Join(absRoot, "screenshots")
		}
		os.MkdirAll(screenshotDir, 0o700)

		bm, err := NewBrowserManager(BrowserConfig{
			ProfileDir:    ts.Browser.ProfileDir,
			ScreenshotDir: screenshotDir,
			Headless:      ts.Browser.Headless,
		})
		if err != nil {
			// Don't fail — browser tools just won't work
			browserMgr = nil
		} else {
			browserMgr = bm
			browserExecs := BrowserExecutors(bm)
			for k, v := range browserExecs {
				m[k] = v
			}
		}
	}

	return m, browserMgr
}

// makeAskUser returns a no-op executor for ask_user.
func makeAskUser() llm.ToolExecutor {
	return makeNoop(AskUserName)
}

// makeNoop returns an executor that returns a provider-intercepted message.
func makeNoop(toolName string) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		return toolName + " is handled by the provider — this should not be reached", false
	}
}
