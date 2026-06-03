package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// humanDelay inserts a random delay to simulate human interaction rhythm.
// Set MCP_CHROME_NO_DELAY=true to disable.
func humanDelay(minMs, maxMs int) {
	if os.Getenv("MCP_CHROME_NO_DELAY") == "true" {
		return
	}
	ms := minMs + rand.IntN(maxMs-minMs+1)
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// ── BrowserManager ────────────────────────────────────────

// BrowserManager manages a Chrome instance with concurrent-safe tab operations.
type BrowserManager struct {
	mu            sync.Mutex
	allocCtx      context.Context
	allocCancel   context.CancelFunc
	tabs          map[string]context.Context
	tabCancel     map[string]context.CancelFunc
	nextTabID     int
	screenshotDir string
}

// BrowserConfig configures the browser manager.
type BrowserConfig struct {
	ProfileDir    string // Chrome profile for persistent cookies/sessions
	ScreenshotDir string // Where screenshots are saved
	Headless      bool   // Run Chrome in headless mode
}

// NewBrowserManager creates a BrowserManager and launches Chrome.
func NewBrowserManager(cfg BrowserConfig) (*BrowserManager, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", cfg.Headless),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.WindowSize(1440, 900),
	)

	if cfg.ProfileDir != "" {
		opts = append(opts, chromedp.UserDataDir(cfg.ProfileDir))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)

	bm := &BrowserManager{
		allocCtx:      allocCtx,
		allocCancel:   allocCancel,
		tabs:          map[string]context.Context{"default": tabCtx},
		tabCancel:     map[string]context.CancelFunc{"default": tabCancel},
		nextTabID:     1,
		screenshotDir: cfg.ScreenshotDir,
	}

	// Health check
	_ = chromedp.Run(tabCtx, chromedp.Evaluate("1+1", nil))

	return bm, nil
}

// Shutdown closes the browser and releases all resources.
func (bm *BrowserManager) Shutdown() {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	for id, cancel := range bm.tabCancel {
		cancel()
		delete(bm.tabs, id)
		delete(bm.tabCancel, id)
	}
	bm.allocCancel()
}

// ── tab management ────────────────────────────────────────

func (bm *BrowserManager) getTab(tabID string) (context.Context, context.CancelFunc) {
	if tabID == "" {
		tabID = "default"
	}
	ctx, ok := bm.tabs[tabID]
	if !ok {
		return bm.tabs["default"], bm.tabCancel["default"]
	}
	return ctx, bm.tabCancel[tabID]
}

func (bm *BrowserManager) execWithTimeout(tabCtx context.Context, timeout time.Duration, actions ...chromedp.Action) error {
	errCh := make(chan error, 1)
	go func() { errCh <- chromedp.Run(tabCtx, actions...) }()
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("browser operation failed: %w", err)
		}
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("operation timed out (%v)", timeout)
	}
}

// ── public operations ─────────────────────────────────────

func (bm *BrowserManager) Navigate(tabID, url string, timeout time.Duration) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	humanDelay(500, 2000)
	tabCtx, _ := bm.getTab(tabID)
	return bm.execWithTimeout(tabCtx, timeout,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	)
}

type browserPageInfo struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	ReadyState string `json:"ready_state"`
}

func (bm *BrowserManager) PageInfo(tabID string) (*browserPageInfo, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	tabCtx, _ := bm.getTab(tabID)
	info := &browserPageInfo{}
	var rs string
	if err := bm.execWithTimeout(tabCtx, 10*time.Second,
		chromedp.Title(&info.Title),
		chromedp.Location(&info.URL),
		chromedp.Evaluate("document.readyState", &rs),
	); err != nil {
		return nil, err
	}
	info.ReadyState = rs
	return info, nil
}

func (bm *BrowserManager) Screenshot(tabID, selector string, fullPage bool, quality int, outputPath string) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	tabCtx, _ := bm.getTab(tabID)
	if outputPath == "" {
		outputPath = filepath.Join(bm.screenshotDir, fmt.Sprintf("screenshot_%d.png", time.Now().UnixNano()))
	}
	var buf []byte
	var actions []chromedp.Action
	if selector != "" {
		actions = append(actions,
			chromedp.WaitVisible(selector, chromedp.ByQuery),
			chromedp.Screenshot(selector, &buf, chromedp.ByQuery),
		)
	} else if fullPage {
		actions = append(actions, chromedp.FullScreenshot(&buf, quality))
	} else {
		actions = append(actions, chromedp.CaptureScreenshot(&buf))
	}
	if err := bm.execWithTimeout(tabCtx, 15*time.Second, actions...); err != nil {
		return "", fmt.Errorf("screenshot failed: %w", err)
	}
	if err := os.WriteFile(outputPath, buf, 0o644); err != nil {
		return "", fmt.Errorf("save screenshot failed: %w", err)
	}
	return outputPath, nil
}

func (bm *BrowserManager) Content(tabID, selector, format string) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	tabCtx, _ := bm.getTab(tabID)
	if selector == "" {
		selector = "body"
	}
	var content string
	var action chromedp.Action
	switch format {
	case "text":
		action = chromedp.Text(selector, &content, chromedp.ByQuery)
	case "outer":
		action = chromedp.OuterHTML(selector, &content, chromedp.ByQuery)
	default:
		action = chromedp.InnerHTML(selector, &content, chromedp.ByQuery)
	}
	if err := bm.execWithTimeout(tabCtx, 10*time.Second, action); err != nil {
		return "", err
	}
	// Truncate to prevent token overflow
	const maxLen = 50000
	if len(content) > maxLen {
		content = content[:maxLen]
		return fmt.Sprintf("(content truncated, showing first %d of %d chars)\n\n%s", maxLen, len(content), content), nil
	}
	return content, nil
}

func (bm *BrowserManager) ExecuteJS(tabID, script string) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	tabCtx, _ := bm.getTab(tabID)
	var result interface{}
	if err := bm.execWithTimeout(tabCtx, 10*time.Second,
		chromedp.Evaluate(script, &result),
	); err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", result), nil
}

func (bm *BrowserManager) Click(tabID, selector string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	humanDelay(200, 800)
	tabCtx, _ := bm.getTab(tabID)
	return bm.execWithTimeout(tabCtx, 10*time.Second,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
	)
}

func (bm *BrowserManager) TypeText(tabID, selector, text string, clear, submit bool) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	humanDelay(300, 900)
	tabCtx, _ := bm.getTab(tabID)
	actions := []chromedp.Action{chromedp.WaitVisible(selector, chromedp.ByQuery)}
	if clear {
		actions = append(actions, chromedp.Evaluate(
			fmt.Sprintf(`document.querySelector('%s').value = ''`, selector), nil,
		))
	}
	actions = append(actions, chromedp.SendKeys(selector, text, chromedp.ByQuery))
	if submit {
		actions = append(actions, chromedp.Evaluate(
			fmt.Sprintf(`var f=document.querySelector('%s').form; if(f) f.submit()`, selector), nil,
		))
	}
	return bm.execWithTimeout(tabCtx, 10*time.Second, actions...)
}

func (bm *BrowserManager) WaitFor(tabID, selector string, timeout time.Duration) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	tabCtx, _ := bm.getTab(tabID)
	return bm.execWithTimeout(tabCtx, timeout,
		chromedp.WaitReady(selector, chromedp.ByQuery),
	)
}

func (bm *BrowserManager) Scroll(tabID string, amount int) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	humanDelay(200, 700)
	tabCtx, _ := bm.getTab(tabID)
	return bm.execWithTimeout(tabCtx, 5*time.Second,
		chromedp.Evaluate(fmt.Sprintf("window.scrollBy(0, %d)", amount), nil),
	)
}

func (bm *BrowserManager) NewTab(url string) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	tabCtx, tabCancel := chromedp.NewContext(bm.allocCtx)
	tabID := fmt.Sprintf("tab_%d", bm.nextTabID)
	bm.nextTabID++
	bm.tabs[tabID] = tabCtx
	bm.tabCancel[tabID] = tabCancel
	if url != "" {
		if err := bm.execWithTimeout(tabCtx, 30*time.Second,
			chromedp.Navigate(url),
			chromedp.WaitReady("body"),
		); err != nil {
			return tabID, fmt.Errorf("tab created but navigation failed: %w", err)
		}
	}
	return tabID, nil
}

func (bm *BrowserManager) CloseTab(tabID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if tabID == "default" || tabID == "" {
		return fmt.Errorf("cannot close default tab")
	}
	cancel, ok := bm.tabCancel[tabID]
	if !ok {
		return fmt.Errorf("tab %s not found", tabID)
	}
	cancel()
	delete(bm.tabs, tabID)
	delete(bm.tabCancel, tabID)
	return nil
}

func (bm *BrowserManager) ListTabs() []string {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	ids := make([]string, 0, len(bm.tabs))
	for id := range bm.tabs {
		ids = append(ids, id)
	}
	return ids
}

// ── executor constructors ─────────────────────────────────

func makeBrowserNavigate(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserNavigateInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		timeout := 30 * time.Second
		if p.TimeoutMs > 0 {
			timeout = time.Duration(p.TimeoutMs) * time.Millisecond
		}
		if err := bm.Navigate(p.TabID, p.URL, timeout); err != nil {
			return err.Error(), true
		}
		info, _ := bm.PageInfo(p.TabID)
		if info != nil {
			return fmt.Sprintf("Navigated to: %s\nTitle: %s\nStatus: %s", info.URL, info.Title, info.ReadyState), false
		}
		return "Navigation done", false
	}
}

func makeBrowserScreenshot(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserScreenshotInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		fullPage := true
		if p.FullPage != nil {
			fullPage = *p.FullPage
		}
		if p.Quality <= 0 {
			p.Quality = 90
		}
		path, err := bm.Screenshot(p.TabID, p.Selector, fullPage, p.Quality, p.OutputPath)
		if err != nil {
			return err.Error(), true
		}
		return fmt.Sprintf("Screenshot saved: %s", path), false
	}
}

func makeBrowserGetContent(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserGetContentInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Format == "" {
			p.Format = "text"
		}
		content, err := bm.Content(p.TabID, p.Selector, p.Format)
		if err != nil {
			return err.Error(), true
		}
		return content, false
	}
}

func makeBrowserExecuteJS(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserExecuteJSInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Script == "" {
			return "script is required", true
		}
		result, err := bm.ExecuteJS(p.TabID, p.Script)
		if err != nil {
			return err.Error(), true
		}
		return result, false
	}
}

func makeBrowserClick(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserClickInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Selector == "" {
			return "selector is required", true
		}
		if err := bm.Click(p.TabID, p.Selector); err != nil {
			return err.Error(), true
		}
		info, _ := bm.PageInfo(p.TabID)
		ctxStr := ""
		if info != nil {
			ctxStr = fmt.Sprintf(" (page: %s)", info.Title)
		}
		return fmt.Sprintf("Clicked: %s%s", p.Selector, ctxStr), false
	}
}

func makeBrowserTypeText(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserTypeTextInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Selector == "" || p.Text == "" {
			return "selector and text are required", true
		}
		if err := bm.TypeText(p.TabID, p.Selector, p.Text, p.Clear, p.Submit); err != nil {
			return err.Error(), true
		}
		msg := fmt.Sprintf("Typed into %s: %s", p.Selector, truncRunes(p.Text, 50))
		if p.Submit {
			msg += " (form submitted)"
		}
		return msg, false
	}
}

func makeBrowserPageInfo(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserPageInfoInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		info, err := bm.PageInfo(p.TabID)
		if err != nil {
			return err.Error(), true
		}
		return fmt.Sprintf("Page: %s\nURL: %s\nStatus: %s", info.Title, info.URL, info.ReadyState), false
	}
}

func makeBrowserWaitFor(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserWaitForInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Selector == "" {
			return "selector is required", true
		}
		timeout := getBrowserTimeout(p.TimeoutMs)
		if err := bm.WaitFor(p.TabID, p.Selector, timeout); err != nil {
			return err.Error(), true
		}
		return fmt.Sprintf("Element ready: %s", p.Selector), false
	}
}

func makeBrowserScroll(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserScrollInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if err := bm.Scroll(p.TabID, p.Amount); err != nil {
			return err.Error(), true
		}
		dir := "down"
		n := p.Amount
		if n < 0 {
			dir = "up"
			n = -n
		}
		return fmt.Sprintf("Scrolled %s %dpx", dir, n), false
	}
}

func makeBrowserManageTab(bm *BrowserManager) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BrowserManageTabInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		switch p.Action {
		case "new":
			tabID, err := bm.NewTab(p.URL)
			if err != nil {
				return err.Error(), true
			}
			return fmt.Sprintf("New tab created: %s", tabID), false
		case "close":
			if p.TabID == "" {
				return "tab_id is required for action='close'", true
			}
			if err := bm.CloseTab(p.TabID); err != nil {
				return err.Error(), true
			}
			return fmt.Sprintf("Tab %s closed", p.TabID), false
		case "list":
			tabs := bm.ListTabs()
			var s string
			for _, id := range tabs {
				info, err := bm.PageInfo(id)
				if err != nil {
					s += fmt.Sprintf("  %s (info unavailable)\n", id)
				} else {
					s += fmt.Sprintf("  %s: %s\n", id, truncRunes(info.Title, 60))
				}
			}
			return "Tabs:\n" + s, false
		default:
			return fmt.Sprintf("unknown action %q (use: new, close, list)", p.Action), true
		}
	}
}

// ── helpers ───────────────────────────────────────────────

func getBrowserTimeout(ms int) time.Duration {
	if ms <= 0 {
		return 10 * time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

func truncRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// ── Browser executor registry ─────────────────────────────

// BrowserExecutors returns executor map for all browser tools.
// Returns nil if bm is nil.
func BrowserExecutors(bm *BrowserManager) map[string]llm.ToolExecutor {
	if bm == nil {
		return nil
	}
	return map[string]llm.ToolExecutor{
		BrowserNavigate:   makeBrowserNavigate(bm),
		BrowserScreenshot: makeBrowserScreenshot(bm),
		BrowserGetContent: makeBrowserGetContent(bm),
		BrowserExecuteJS:  makeBrowserExecuteJS(bm),
		BrowserClick:      makeBrowserClick(bm),
		BrowserTypeText:   makeBrowserTypeText(bm),
		BrowserPageInfo:   makeBrowserPageInfo(bm),
		BrowserWaitFor:    makeBrowserWaitFor(bm),
		BrowserScroll:     makeBrowserScroll(bm),
		BrowserManageTab:  makeBrowserManageTab(bm),
	}
}
