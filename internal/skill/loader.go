package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
	"gopkg.in/yaml.v3"
)

// SkillDef is the frontmatter metadata parsed from a skill.md file.
// The prompt body is everything after the closing "---".
type SkillDef struct {
	// Name is the unique skill identifier (e.g. "code_review").
	// Defaults to the directory name if omitted.
	Name string `yaml:"name"`

	// Description is a one-line summary shown to the model.
	Description string `yaml:"description"`

	// Enabled controls whether the skill is loaded. Set to false
	// to disable without deleting the file. Default: true.
	Enabled *bool `yaml:"enabled,omitempty"`
}

// defSkill adapts a SkillDef + prompt body to the Skill interface.
type defSkill struct {
	def    SkillDef
	prompt string
}

func (s defSkill) Name() string                           { return s.def.Name }
func (s defSkill) Description() string                    { return s.def.Description }
func (s defSkill) Prompt() string                         { return s.prompt }
func (s defSkill) Tools() []llm.Tool                      { return nil }
func (s defSkill) Executors() map[string]llm.ToolExecutor { return nil }

// LoadFromDir scans dir for skill subdirectories and loads all enabled
// skills into the registry. Each subdirectory must contain a "skill.md"
// file with YAML frontmatter.
//
// Subdirectories starting with "." or "_" are skipped.
//
// Example skill.md:
//
//	---
//	name: my_skill
//	description: Does something useful
//	---
//	## My Skill
//
//	Instructions for the model go here...
func LoadFromDir(reg *Registry, dir string) (loaded int, _ error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read skills dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if len(dirName) == 0 || dirName[0] == '.' || dirName[0] == '_' {
			continue // skip hidden / disabled-prefix dirs
		}

		def, prompt, err := loadSkillMD(filepath.Join(dir, dirName))
		if err != nil {
			return loaded, fmt.Errorf("skill %s: %w", dirName, err)
		}
		if def == nil {
			continue // no skill.md found
		}

		if def.Enabled != nil && !*def.Enabled {
			continue // explicitly disabled
		}
		if def.Name == "" {
			def.Name = dirName
		}

		if err := reg.Load(defSkill{def: *def, prompt: prompt}); err != nil {
			return loaded, fmt.Errorf("load skill %s: %w", def.Name, err)
		}
		loaded++
	}

	return loaded, nil
}

// loadSkillMD reads a skill.md file and parses its frontmatter + body.
// Returns nil, "", nil if skill.md doesn't exist.
func loadSkillMD(dir string) (*SkillDef, string, error) {
	path := filepath.Join(dir, "skill.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("read skill.md: %w", err)
	}

	fm, body := parseFrontmatter(string(data))
	if fm == "" {
		return nil, "", fmt.Errorf("skill.md must start with YAML frontmatter between --- markers")
	}

	var def SkillDef
	if err := yaml.Unmarshal([]byte(fm), &def); err != nil {
		return nil, "", fmt.Errorf("parse frontmatter: %w", err)
	}

	return &def, strings.TrimSpace(body), nil
}

// parseFrontmatter extracts YAML frontmatter from markdown content.
// Frontmatter is the text between the first two "---" lines.
// Returns (frontmatter, body).
func parseFrontmatter(content string) (string, string) {
	content = strings.TrimLeft(content, "\r\n")
	if !strings.HasPrefix(content, "---") {
		return "", content
	}

	// Find end of opening "---"
	idx := strings.Index(content[3:], "\n---")
	if idx < 0 {
		return "", content
	}

	fm := strings.TrimSpace(content[3 : 3+idx])
	body := content[3+idx+4:] // skip past "\n---"
	return fm, body
}
