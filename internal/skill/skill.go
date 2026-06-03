// Package skill provides a pluggable skill system for the GoDFreeCLI agent.
//
// A Skill is a self-contained capability module that bundles:
//   - A system prompt fragment (injected when the skill is active)
//   - Additional tool definitions (registered into the tool registry)
//   - Tool executor implementations
//
// Skills are activated via the use_skill tool. The model calls
// use_skill(name="...") and the provider intercepts it, emitting a
// UseSkillEvent. The consumer then activates the skill and re-enters
// the ReAct loop with the enhanced setup.
package skill

import (
	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// Skill is a pluggable capability that can be loaded on demand.
// Implementations register themselves with the SkillRegistry.
type Skill interface {
	// Name returns the unique skill identifier (e.g. "code_review").
	// This is the value the model passes to use_skill(name="...").
	Name() string

	// Description returns a one-line summary shown to the user and model.
	Description() string

	// Prompt returns additional system prompt content injected when
	// the skill is active. The returned text is appended to the
	// existing system prompt.
	Prompt() string

	// Tools returns additional tool definitions provided by this skill.
	// These are registered into the tool registry when the skill activates.
	// Return nil or empty if the skill adds no tools.
	Tools() []llm.Tool

	// Executors returns tool implementations for this skill's tools,
	// keyed by tool name. These are registered into the global executor
	// map when the skill activates.
	Executors() map[string]llm.ToolExecutor
}

// Info is a lightweight representation of a skill for listing.
type Info struct {
	Name        string
	Description string
	PromptLen   int
	ToolCount   int
}

// SkillInfo extracts Info from a Skill.
func SkillInfo(s Skill) Info {
	tools := s.Tools()
	return Info{
		Name:        s.Name(),
		Description: s.Description(),
		PromptLen:   len(s.Prompt()),
		ToolCount:   len(tools),
	}
}
