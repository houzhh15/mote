package cfg

// StepType 定义步骤类型（4 种，递归通过 route target 指向自身实现）
type StepType string

const (
	StepPrompt   StepType = "prompt"    // 终结符：LLM 执行具体提示词
	StepAgentRef StepType = "agent_ref" // 非终结符：引用另一个 Agent
	StepRoute    StepType = "route"     // 路由：LLM 判断后选择目标 Agent 分支
	StepExec     StepType = "exec"      // 合成步骤：无 Steps 代理走完整 orchestrator 循环
)

// RouteEndMarker is a special branch target value that terminates the route
// step without pushing a new frame. When a route's resolved target equals
// this marker, the step completes normally and execution advances to the
// next step (or the frame pops if it was the last step).
// Usage in config:
//
//	branches:
//	  结束: _end
//	  _default: _end
const RouteEndMarker = "_end"

// Step 表示产生式中的一个符号
type Step struct {
	Type StepType `yaml:"type" json:"type"`

	// prompt 类型：LLM 执行的具体指令
	Content string `yaml:"content,omitempty" json:"content,omitempty"`

	// agent_ref 类型：引用的 Agent 名称
	Agent string `yaml:"agent,omitempty" json:"agent,omitempty"`

	// route 类型：LLM 路由判断
	Prompt   string            `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Branches map[string]string `yaml:"branches,omitempty" json:"branches,omitempty"` // match → target agent name

	// 通用
	Label string `yaml:"label,omitempty" json:"label,omitempty"` // 可选标签，用于 UI 展示
}
