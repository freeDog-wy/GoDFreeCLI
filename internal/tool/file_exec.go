package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// ── helpers ───────────────────────────────────────────────

// resolvePath resolves p against root. If p is absolute, it is returned
// as-is (cleaned). Otherwise it is joined with root.
func resolvePath(root, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(root, p)
}

// ensureParentDir creates all parent directories for the given file path.
func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o755)
}

// ── read_file ─────────────────────────────────────────────

func makeReadFile(root string) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p ReadFileInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Path == "" {
			return "path is required", true
		}

		path := resolvePath(root, p.Path)

		f, err := os.Open(path)
		if err != nil {
			return fmt.Sprintf("cannot open %s: %v", path, err), true
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			return fmt.Sprintf("cannot stat %s: %v", path, err), true
		}
		if info.IsDir() {
			return fmt.Sprintf("%s is a directory, not a file", path), true
		}

		var lines []string
		scanner := bufio.NewScanner(f)
		// Allow long lines
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return fmt.Sprintf("error reading %s: %v", path, err), true
		}

		// Apply offset/limit
		start := 0
		if p.Offset > 0 {
			start = p.Offset - 1 // convert to 0-indexed
			if start >= len(lines) {
				start = len(lines)
			}
		}
		end := len(lines)
		if p.Limit > 0 {
			if end := start + p.Limit; end < len(lines) {
				end = start + p.Limit
			}
		}
		lines = lines[start:end]
		lineNumBase := start + 1

		var sb strings.Builder
		for i, line := range lines {
			fmt.Fprintf(&sb, "%6d\t%s\n", lineNumBase+i, line)
		}

		if sb.Len() == 0 {
			return "(file is empty)", false
		}
		return sb.String(), false
	}
}

// ── write_file ────────────────────────────────────────────

func makeWriteFile(root string) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p WriteFileInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Path == "" {
			return "path is required", true
		}
		if p.Content == "" && !strings.HasSuffix(p.Path, "/") {
			// Allow empty content (e.g. creating empty file), but warn
		}

		path := resolvePath(root, p.Path)

		if err := ensureParentDir(path); err != nil {
			return fmt.Sprintf("cannot create parent dirs for %s: %v", path, err), true
		}

		if err := os.WriteFile(path, []byte(p.Content), 0o644); err != nil {
			return fmt.Sprintf("cannot write %s: %v", path, err), true
		}

		return fmt.Sprintf("Wrote %d bytes to %s", len(p.Content), path), false
	}
}

// ── list_files ────────────────────────────────────────────

func makeListFiles(root string) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p ListFilesInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}

		path := root
		if p.Path != "" {
			path = resolvePath(root, p.Path)
		}

		depth := p.Depth
		if depth <= 0 {
			depth = 1
		}
		if depth > 5 {
			depth = 5
		}

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Sprintf("cannot access %s: %v", path, err), true
		}
		if !info.IsDir() {
			// Single file — just return it
			return fmt.Sprintf("%s  (%s)", path, humanSize(info.Size())), false
		}

		var entries []string
		filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip inaccessible entries
			}
			if p == path {
				return nil
			}

			// Calculate depth relative to base
			rel, _ := filepath.Rel(path, p)
			curDepth := len(strings.Split(rel, string(filepath.Separator)))
			if curDepth > depth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			prefix := ""
			if d.IsDir() {
				prefix = "/"
			}

			entryInfo, err := d.Info()
			size := ""
			if err == nil && !d.IsDir() {
				size = "  " + humanSize(entryInfo.Size())
			}

			entries = append(entries, fmt.Sprintf("%s%s%s", rel, prefix, size))
			return nil
		})

		sort.Strings(entries)

		if len(entries) == 0 {
			return fmt.Sprintf("%s (empty directory)", path), false
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "%s (%d entries):\n", path, len(entries))
		for _, e := range entries {
			sb.WriteString(e)
			sb.WriteByte('\n')
		}
		return sb.String(), false
	}
}

// ── search_file ───────────────────────────────────────────

func makeSearchFile(root string) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p SearchFileInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Pattern == "" {
			return "pattern is required", true
		}

		base := root
		if p.Path != "" {
			base = resolvePath(root, p.Path)
		}

		// Build full glob pattern
		fullPattern := filepath.Join(base, p.Pattern)

		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			return fmt.Sprintf("invalid pattern: %v", err), true
		}

		// Also try recursive glob with **
		if strings.Contains(p.Pattern, "**") {
			filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(base, path)
				if ok, _ := filepath.Match(p.Pattern, rel); ok {
					// Check if we already have this match
					alreadyHave := false
					for _, m := range matches {
						if m == path {
							alreadyHave = true
							break
						}
					}
					if !alreadyHave {
						matches = append(matches, path)
					}
				}
				return nil
			})
		}

		if len(matches) == 0 {
			return fmt.Sprintf("no files matching %s", p.Pattern), false
		}

		// Sort by modification time (newest first)
		sort.Slice(matches, func(i, j int) bool {
			ii, _ := os.Stat(matches[i])
			jj, _ := os.Stat(matches[j])
			if ii == nil || jj == nil {
				return matches[i] < matches[j]
			}
			return ii.ModTime().After(jj.ModTime())
		})

		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d files matching %s:\n", len(matches), p.Pattern)
		for _, m := range matches {
			rel, _ := filepath.Rel(base, m)
			if rel == "" {
				rel = m
			}
			info, _ := os.Stat(m)
			if info != nil && !info.IsDir() {
				fmt.Fprintf(&sb, "  %s  (%s)\n", rel, humanSize(info.Size()))
			} else {
				fmt.Fprintf(&sb, "  %s/\n", rel)
			}
		}
		return sb.String(), false
	}
}

// ── search_content ────────────────────────────────────────

func makeGrep(root string) llm.ToolExecutor {
	return func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
		var p GrepInput
		if err := json.Unmarshal(input, &p); err != nil {
			return fmt.Sprintf("invalid input: %v", err), true
		}
		if p.Pattern == "" {
			return "pattern is required", true
		}

		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return fmt.Sprintf("invalid regex: %v", err), true
		}

		base := root
		if p.Path != "" {
			base = resolvePath(root, p.Path)
		}

		outputMode := p.OutputMode
		if outputMode == "" {
			outputMode = "files_with_matches"
		}
		headLimit := p.HeadLimit
		if headLimit <= 0 {
			headLimit = 250
		}

		type match struct {
			file string
			line int
			text string
		}

		var matches []match
		fileMatchSet := make(map[string]bool)
		countByFile := make(map[string]int)

		filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}

			// Glob filter
			if p.Glob != "" {
				rel, _ := filepath.Rel(base, path)
				ok, _ := filepath.Match(p.Glob, filepath.Base(rel))
				if !ok {
					// Also try matching against full relative path
					ok2, _ := filepath.Match(p.Glob, rel)
					if !ok2 {
						return nil
					}
				}
			}

			// Skip binary/large files (>10MB)
			info, _ := d.Info()
			if info != nil && info.Size() > 10*1024*1024 {
				return nil
			}

			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				if re.MatchString(line) {
					rel, _ := filepath.Rel(base, path)
					if rel == "" {
						rel = path
					}
					matches = append(matches, match{file: rel, line: lineNum, text: line})
					fileMatchSet[rel] = true
					countByFile[rel]++
					if len(matches) >= headLimit {
						break
					}
				}
			}
			if len(matches) >= headLimit {
				return filepath.SkipAll
			}
			if scanner.Err() != nil {
				return scanner.Err()
			}
			return nil
		})

		switch outputMode {
		case "count":
			files := make([]string, 0, len(countByFile))
			for f := range countByFile {
				files = append(files, f)
			}
			sort.Strings(files)
			var sb strings.Builder
			for _, f := range files {
				fmt.Fprintf(&sb, "%s: %d matches\n", f, countByFile[f])
			}
			if sb.Len() == 0 {
				return "no matches", false
			}
			return sb.String(), false

		case "files_with_matches":
			files := make([]string, 0, len(fileMatchSet))
			for f := range fileMatchSet {
				files = append(files, f)
			}
			sort.Strings(files)
			if len(files) == 0 {
				return "no matches", false
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "Found %d files matching %q:\n", len(files), p.Pattern)
			for _, f := range files {
				fmt.Fprintf(&sb, "  %s\n", f)
			}
			return sb.String(), false

		default: // "content"
			if len(matches) == 0 {
				return "no matches", false
			}
			var sb strings.Builder
			for _, m := range matches {
				fmt.Fprintf(&sb, "%s:%d: %s\n", m.file, m.line, m.text)
			}
			if len(matches) >= headLimit {
				fmt.Fprintf(&sb, "... (truncated at %d matches)\n", headLimit)
			}
			return sb.String(), false
		}
	}
}

// ── humanSize ─────────────────────────────────────────────

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGT"[exp])
}
