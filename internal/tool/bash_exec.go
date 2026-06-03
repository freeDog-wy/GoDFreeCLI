package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// ── execute_bash ──────────────────────────────────────────

func makeBash(root string) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p BashInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Command == "" {
			return "command is required", true
		}

		// Working directory
		workdir := root
		if p.Workdir != "" {
			workdir = resolvePath(root, p.Workdir)
		}

		// Timeout
		timeout := time.Duration(p.TimeoutMs) * time.Millisecond
		if timeout <= 0 {
			timeout = 120 * time.Second // default 2 min
		}
		if timeout > 10*time.Minute {
			timeout = 10 * time.Minute // max 10 min
		}

		execCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Build shell command
		var shell string
		var shellArgs []string
		if runtime.GOOS == "windows" {
			shell = "cmd"
			shellArgs = []string{"/c", p.Command}
		} else {
			shell = "sh"
			shellArgs = []string{"-c", p.Command}
		}

		cmd := exec.CommandContext(execCtx, shell, shellArgs...)
		cmd.Dir = workdir
		cmd.Env = os.Environ()

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		startTime := time.Now()
		err := cmd.Run()
		elapsed := time.Since(startTime)

		result := BashResult{
			Stdout: stdout.String(),
			Stderr: stderr.String(),
		}

		if err != nil {
			if execCtx.Err() == context.DeadlineExceeded {
				result.ExitCode = -1
				result.TimedOut = true
			} else if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = -1
				result.Stderr += fmt.Sprintf("\n[error] %v", err)
			}
		}

		// Build human-readable output
		summary := fmt.Sprintf("exit=%d  elapsed=%.1fs", result.ExitCode, elapsed.Seconds())
		if result.TimedOut {
			summary += "  TIMED_OUT"
		}

		var out string
		if result.Stdout != "" && result.Stderr != "" {
			out = fmt.Sprintf("[stdout]\n%s\n[stderr]\n%s\n[%s]",
				result.Stdout, result.Stderr, summary)
		} else if result.Stderr != "" {
			out = fmt.Sprintf("[stderr]\n%s\n[%s]", result.Stderr, summary)
		} else if result.Stdout != "" {
			out = fmt.Sprintf("[stdout]\n%s\n[%s]", result.Stdout, summary)
		} else {
			out = fmt.Sprintf("[no output] [%s]", summary)
		}

		// Treat non-zero exit as an error so the model can
		// react to failures (but still include all output).
		isError := result.ExitCode != 0

		return out, isError
	}
}
