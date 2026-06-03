package tool

import "github.com/freeDog-wy/GoDFreeCLI/internal/llm"

// ── File tool names ──

const (
	ReadFileName   = "read_file"
	WriteFileName  = "write_file"
	ListFileName   = "list_files"
	SearchFileName = "search_file"
	GrepFileName   = "search_content"
)

// ── read_file ─────────────────────────────────────────────

// ReadFileTool returns the read_file tool definition.
//
// The model uses this to read file contents. The executor should
// resolve relative paths against the project root and enforce
// sandbox boundaries.
func ReadFileTool() llm.Tool {
	return llm.Tool{
		Name:        ReadFileName,
		Description: "Reads a file from the local filesystem. Returns the file content with line numbers. Use this to inspect files before editing them or to understand existing code.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"path": {
					Type:        "string",
					Description: "The absolute or relative path to the file. Relative paths are resolved against the project root.",
				},
				"offset": {
					Type:        "integer",
					Description: "Optional. Line number to start reading from (1-based). Use for large files.",
				},
				"limit": {
					Type:        "integer",
					Description: "Optional. Maximum number of lines to read. Use for large files.",
				},
			},
			Required: []string{"path"},
		},
	}
}

// ReadFileInput is a typed representation of read_file arguments.
type ReadFileInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// ── write_file ────────────────────────────────────────────

// WriteFileTool returns the write_file tool definition.
//
// The model uses this to create or overwrite a file. The executor
// should enforce sandbox boundaries and confirm before overwriting
// existing files.
func WriteFileTool() llm.Tool {
	return llm.Tool{
		Name:        WriteFileName,
		Description: "Creates a new file or overwrites an existing file with the given content. Use this to write generated code, configuration, or documentation to disk.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"path": {
					Type:        "string",
					Description: "The absolute or relative path for the file. Parent directories are created automatically if they don't exist.",
				},
				"content": {
					Type:        "string",
					Description: "The complete content to write to the file.",
				},
			},
			Required: []string{"path", "content"},
		},
	}
}

// WriteFileInput is a typed representation of write_file arguments.
type WriteFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ── list_files ────────────────────────────────────────────

// ListFilesTool returns the list_files tool definition.
//
// The model uses this to explore the directory structure. The executor
// should return file paths relative to the listed directory.
func ListFilesTool() llm.Tool {
	return llm.Tool{
		Name:        ListFileName,
		Description: "Lists files and directories in a given directory. Use this to explore the project structure, understand code organization, or find files when you don't know the exact path.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"path": {
					Type:        "string",
					Description: "The directory path to list. Defaults to the project root if omitted.",
				},
				"depth": {
					Type:        "integer",
					Description: "Optional. Recursion depth for listing. 1 means only the given directory. Default: 1. Max: 5.",
				},
			},
			Required: []string{},
		},
	}
}

// ListFilesInput is a typed representation of list_files arguments.
type ListFilesInput struct {
	Path  string `json:"path,omitempty"`
	Depth int    `json:"depth,omitempty"`
}

// ── search_file (glob) ────────────────────────────────────

// SearchFileTool returns the search_file tool definition.
//
// The model uses this to find files by glob pattern. This is a
// fast file-name-only search — use search_content for content searches.
func SearchFileTool() llm.Tool {
	return llm.Tool{
		Name:        SearchFileName,
		Description: "Searches for files matching a glob pattern. Returns matching file paths sorted by modification time. Use this to find files by name or extension (e.g. '**/*.go', '*.md'). This is a filename-only search — use search_content to search inside files.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"pattern": {
					Type:        "string",
					Description: "The glob pattern to match against file paths. Supports ** for recursive matching. Examples: '**/*.go', 'src/**/*.ts', '*.md'.",
				},
				"path": {
					Type:        "string",
					Description: "Optional. The directory to search in. Defaults to the project root.",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

// SearchFileInput is a typed representation of search_file arguments.
type SearchFileInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// ── search_content (grep) ─────────────────────────────────

// GrepTool returns the search_content tool definition.
//
// The model uses this to search file contents with regex patterns.
// This corresponds to ripgrep/grep functionality.
func GrepTool() llm.Tool {
	return llm.Tool{
		Name:        GrepFileName,
		Description: "Searches file contents using a regular expression pattern. Returns matching lines with file paths and line numbers. Use this to find code patterns, function definitions, error messages, or any text inside files. Prefer this over execute_bash with grep — it's faster and respects .gitignore.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"pattern": {
					Type:        "string",
					Description: "The regular expression pattern to search for. Uses ripgrep syntax. Examples: 'func\\s+\\w+', 'import.*errors', 'TODO'.",
				},
				"path": {
					Type:        "string",
					Description: "Optional. File or directory to search in. Defaults to the project root.",
				},
				"glob": {
					Type:        "string",
					Description: "Optional. Glob pattern to filter files (e.g. '*.go', '*.{ts,tsx}'). Maps to ripgrep --glob.",
				},
				"output_mode": {
					Type:        "string",
					Description: "Optional. How to return results. 'content' shows matching lines, 'files_with_matches' shows file paths only, 'count' shows match counts. Default: 'files_with_matches'.",
					Enum:        []string{"content", "files_with_matches", "count"},
				},
				"head_limit": {
					Type:        "integer",
					Description: "Optional. Limit output to first N matches. Default: 250.",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

// GrepInput is a typed representation of search_content arguments.
type GrepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`
	OutputMode string `json:"output_mode,omitempty"`
	HeadLimit  int    `json:"head_limit,omitempty"`
}

// ── All file tools helper ─────────────────────────────────

// FileTools returns all built-in file operation tools as a slice.
// Convenience function for registering all file tools at once.
func FileTools() []llm.Tool {
	return []llm.Tool{
		ReadFileTool(),
		WriteFileTool(),
		ListFilesTool(),
		SearchFileTool(),
		GrepTool(),
	}
}
