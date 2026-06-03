package skill

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/freeDog-wy/GoDFreeCLI/internal/registry"
)

// Registry manages all loaded skills and handles activation.
// Skills are registered at startup. The model activates them on demand
// via the use_skill tool.
type Registry struct {
	mu      sync.RWMutex
	skills  map[string]Skill // loaded skills
	toolReg *registry.Registry
	active  map[string]bool // currently active skill names
}

// NewRegistry creates a skill registry that registers skill tools
// into the given tool registry on activation.
func NewRegistry(toolReg *registry.Registry) *Registry {
	return &Registry{
		skills:  make(map[string]Skill),
		toolReg: toolReg,
		active:  make(map[string]bool),
	}
}

// Load adds a skill to the registry. Skills are not active until the
// model calls use_skill(name="...").
func (r *Registry) Load(s Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := s.Name()
	if _, ok := r.skills[name]; ok {
		return fmt.Errorf("skill %q already loaded", name)
	}
	r.skills[name] = s
	return nil
}

// Activate enables a skill: its prompt is appended to the conversation's
// system prompt and its tools are registered. Activate is idempotent —
// activating the same skill twice is a no-op.
//
// Returns the merged prompt fragment and any error from tool registration.
func (r *Registry) Activate(name string) (prompt string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.active[name] {
		return "", nil // already active
	}

	s, ok := r.skills[name]
	if !ok {
		return "", fmt.Errorf("unknown skill: %q (available: %s)", name, r.listLocked())
	}

	// Register skill tools
	tools := s.Tools()
	execs := s.Executors()
	if len(tools) > 0 {
		if err := r.toolReg.RegisterAll(tools, execs); err != nil {
			return "", fmt.Errorf("register tools for skill %q: %w", name, err)
		}
	}

	r.active[name] = true
	return s.Prompt(), nil
}

// IsActive reports whether a skill is currently active.
func (r *Registry) IsActive(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active[name]
}

// ActivePrompt returns the concatenated prompts of all active skills.
func (r *Registry) ActivePrompt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var parts []string
	for name := range r.active {
		if s := r.skills[name]; s != nil {
			if p := s.Prompt(); p != "" {
				parts = append(parts, p)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// List returns Info for all loaded skills.
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.skills))
	for n := range r.skills {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]Info, 0, len(names))
	for _, n := range names {
		out = append(out, SkillInfo(r.skills[n]))
	}
	return out
}

// ListPrompt returns a formatted description of all loaded skills,
// suitable for injection into the system prompt so the model knows
// which skills are available.
func (r *Registry) ListPrompt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Skills\n\n")
	b.WriteString("Use the `use_skill` tool to activate a skill. Once activated, the skill's instructions and tools become available.\n\n")

	names := make([]string, 0, len(r.skills))
	for n := range r.skills {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		s := r.skills[n]
		fmt.Fprintf(&b, "- **%s**: %s\n", s.Name(), s.Description())
	}
	return b.String()
}

func (r *Registry) listLocked() string {
	names := make([]string, 0, len(r.skills))
	for n := range r.skills {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
