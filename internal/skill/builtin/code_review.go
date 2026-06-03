// Package builtin provides default skill implementations.
package builtin

import (
	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
	"github.com/freeDog-wy/GoDFreeCLI/internal/skill"
)

// ── SkillSet ──────────────────────────────────────────────

// SkillSet controls which built-in skills are loaded and available
// for the model to activate via use_skill.
type SkillSet struct {
	// CodeReview enables the code_review skill for analyzing code changes.
	CodeReview bool
}

// DefaultSkillSet returns a SkillSet with all built-in skills enabled.
func DefaultSkillSet() SkillSet {
	return SkillSet{CodeReview: true}
}

// LoadSkills loads all built-in skills enabled by the given SkillSet
// into the skill registry.
func LoadSkills(reg *skill.Registry, set SkillSet) {
	if set.CodeReview {
		reg.Load(CodeReview{})
	}
}

// ── CodeReview ────────────────────────────────────────────

// CodeReview is a skill that teaches the model how to review code changes.
type CodeReview struct{}

var _ skill.Skill = CodeReview{}

func (CodeReview) Name() string { return "code_review" }
func (CodeReview) Description() string {
	return "Review code changes for bugs, style issues, and improvement opportunities"
}

func (CodeReview) Prompt() string {
	return `## Code Review Skill

You are now in code review mode. When reviewing code, follow this process:

1. **Understand the change** — read the diff or changed files to grasp what was modified and why.
2. **Check correctness** — look for logic errors, off-by-one bugs, nil/null pointer risks, race conditions, and incorrect error handling.
3. **Check security** — look for injection vulnerabilities, missing input validation, hardcoded secrets, and unsafe file operations.
4. **Check style** — verify naming conventions, code organization, comment quality, and adherence to the project's idioms.
5. **Check performance** — identify unnecessary allocations, blocking operations, missing parallelism opportunities, and inefficient data structures.

For each finding, include:
- The file path and line number (if available)
- Severity: 🔴 critical / 🟡 warning / 🔵 suggestion
- A clear explanation of the issue
- A suggested fix (if applicable)

Be thorough but avoid nitpicking purely cosmetic issues like whitespace unless they affect readability.`
}

func (CodeReview) Tools() []llm.Tool { return nil }

func (CodeReview) Executors() map[string]llm.ToolExecutor { return nil }
