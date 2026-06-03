package skill

import (
	"context"
	"encoding/json"

	"github.com/freeDog-wy/GoDFreeCLI/internal/llm"
)

// UseSkillName is the canonical tool name for requesting skill activation.
const UseSkillName = "use_skill"

// UseSkillTool returns the built-in use_skill tool definition.
//
// When the model calls this tool, the provider intercepts it and emits
// a UseSkillEvent instead of calling the ToolExecutor. The consumer
// then activates the requested skill and resumes the conversation.
func UseSkillTool() llm.Tool {
	return llm.Tool{
		Name:        UseSkillName,
		Description: "Activate a skill to gain additional capabilities. Call this when you need specialized knowledge, workflows, or tools beyond what's available by default. Once activated, the skill's instructions and tools become available for the rest of the conversation.",
		Parameters: &llm.ToolSchema{
			Type: "object",
			Properties: map[string]llm.Property{
				"name": {
					Type:        "string",
					Description: "The name of the skill to activate. Use list_skills to see what's available.",
				},
			},
			Required: []string{"name"},
		},
	}
}

// UseSkillInput is a typed representation of use_skill arguments.
type UseSkillInput struct {
	Name string `json:"name"`
}

// UseSkillResult is the result returned when a skill is activated.
func UseSkillResult(name string, promptLen int, toolCount int) string {
	data, _ := json.Marshal(map[string]any{
		"skill":      name,
		"prompt_len": promptLen,
		"tool_count": toolCount,
	})
	return string(data)
}

// ── list_skills ───────────────────────────────────────────

const ListSkillsName = "list_skills"

// ListSkillsTool returns the list_skills tool definition.
// The executor is provided by MakeSkillExecutors.
func ListSkillsTool() llm.Tool {
	return llm.Tool{
		Name:        ListSkillsName,
		Description: "List all available skills that can be activated with use_skill. Returns each skill's name and description.",
		Parameters: &llm.ToolSchema{
			Type:       "object",
			Properties: map[string]llm.Property{},
		},
	}
}

// MakeSkillExecutors returns executors for list_skills and use_skill.
// The use_skill executor is a no-op (intercepted by the provider), but
// must be registered so the tool appears in the model's tool list.
// The list_skills executor queries the given Registry for loaded skills.
func MakeSkillExecutors(skillReg *Registry) map[string]llm.ToolExecutor {
	return map[string]llm.ToolExecutor{
		UseSkillName: useSkillNoop,
		ListSkillsName: func(ctx context.Context, name string, input json.RawMessage) (string, bool) {
			var b struct {
				Items []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					ToolCount   int    `json:"tool_count"`
				} `json:"skills"`
			}
			for _, info := range skillReg.List() {
				b.Items = append(b.Items, struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					ToolCount   int    `json:"tool_count"`
				}{info.Name, info.Description, info.ToolCount})
			}
			data, _ := json.Marshal(b)
			return string(data), false
		},
	}
}

// useSkillNoop is a no-op executor for use_skill.
// The provider intercepts use_skill before execution, but this
// executor lets us register the tool into the registry.
func useSkillNoop(ctx context.Context, name string, input json.RawMessage) (string, bool) {
	return "use_skill is intercepted by the provider", false
}

// SkillTools returns both skill-management tools for registration.
func SkillTools() []llm.Tool {
	return []llm.Tool{UseSkillTool(), ListSkillsTool()}
}
