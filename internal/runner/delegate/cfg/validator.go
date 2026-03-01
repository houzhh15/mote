package cfg

import "fmt"

// ValidationLevel 验证结果级别
type ValidationLevel int

const (
	LevelError   ValidationLevel = iota // 阻断性错误，不可执行
	LevelWarning                        // 警告，可执行但可能有问题
)

// ValidationResult 单条验证结果
type ValidationResult struct {
	Level     ValidationLevel `json:"level"`
	Code      string          `json:"code"`
	Message   string          `json:"message"`
	AgentName string          `json:"agent_name"`
	StepIndex int             `json:"step_index"` // -1 表示全局
}

// AgentLookup 查询 Agent 是否存在及其步骤配置。
// 返回 (steps, exists)。steps 为 nil 表示 Agent 存在但无结构化步骤。
type AgentLookup func(name string) (steps []Step, exists bool)

// Validator CFG 验证器
type Validator struct{}

// NewValidator 创建验证器实例
func NewValidator() *Validator {
	return &Validator{}
}

// Validate 验证 Agent 步骤配置的正确性
func (v *Validator) Validate(
	agentName string,
	steps []Step,
	maxRecursion int,
	lookup AgentLookup,
) []ValidationResult {
	var results []ValidationResult

	// R1: 检查空步骤
	if len(steps) == 0 {
		results = append(results, ValidationResult{
			Level: LevelError, Code: "EMPTY_STEPS",
			Message:   "agent declares steps but list is empty",
			AgentName: agentName, StepIndex: -1,
		})
		return results
	}

	// R2: 检查 agent_ref 引用的 Agent 是否存在
	results = append(results, v.checkAgentRefs(agentName, steps, lookup)...)

	// R3: 检查 route 分支完整性与 target 有效性
	results = append(results, v.checkRoutes(agentName, steps, lookup)...)

	// R4: 检查递归（route target 指向自身）是否有 max_recursion 限制
	results = append(results, v.checkSelfRouteRecursion(agentName, steps, maxRecursion)...)

	// R5: 检查循环依赖（A→B→A 且双方均无递归限制）
	results = append(results, v.checkCyclicDeps(agentName, steps, lookup)...)

	return results
}

// checkAgentRefs 验证 agent_ref 步骤引用的 Agent 是否存在
func (v *Validator) checkAgentRefs(
	agentName string,
	steps []Step,
	lookup AgentLookup,
) []ValidationResult {
	var results []ValidationResult
	for i, step := range steps {
		if step.Type != StepAgentRef {
			continue
		}
		if _, exists := lookup(step.Agent); !exists {
			results = append(results, ValidationResult{
				Level: LevelError, Code: "MISSING_AGENT_REF",
				Message:   fmt.Sprintf("agent_ref step references unknown agent %q", step.Agent),
				AgentName: agentName, StepIndex: i,
			})
		}
	}
	return results
}

// checkRoutes 验证 route 步骤
func (v *Validator) checkRoutes(
	agentName string,
	steps []Step,
	lookup AgentLookup,
) []ValidationResult {
	var results []ValidationResult
	for i, step := range steps {
		if step.Type != StepRoute {
			continue
		}
		// 检查 branches 非空
		if len(step.Branches) == 0 {
			results = append(results, ValidationResult{
				Level: LevelError, Code: "EMPTY_ROUTE_BRANCHES",
				Message:   "route step has no branches",
				AgentName: agentName, StepIndex: i,
			})
			continue
		}
		// 检查每个 target agent 是否存在
		hasDefault := false
		for match, target := range step.Branches {
			if match == "_default" {
				hasDefault = true
			}
			// target == agentName 是自引用递归，当前 agent 一定存在，跳过检查
			if target == agentName {
				continue
			}
			// _end is a special marker that terminates the route step
			if target == RouteEndMarker {
				continue
			}
			if _, exists := lookup(target); !exists {
				results = append(results, ValidationResult{
					Level: LevelError, Code: "ROUTE_TARGET_NOT_FOUND",
					Message:   fmt.Sprintf("route branch %q targets unknown agent %q", match, target),
					AgentName: agentName, StepIndex: i,
				})
			}
		}
		if !hasDefault {
			results = append(results, ValidationResult{
				Level: LevelWarning, Code: "MISSING_DEFAULT_ROUTE",
				Message:   "route step has no _default branch",
				AgentName: agentName, StepIndex: i,
			})
		}
	}
	return results
}

// checkSelfRouteRecursion 检查 route 分支是否指向自身且无递归限制
func (v *Validator) checkSelfRouteRecursion(
	agentName string,
	steps []Step,
	maxRecursion int,
) []ValidationResult {
	var results []ValidationResult
	for i, step := range steps {
		if step.Type != StepRoute {
			continue
		}
		for _, target := range step.Branches {
			if target == agentName && maxRecursion <= 0 {
				results = append(results, ValidationResult{
					Level: LevelError, Code: "SELF_ROUTE_NO_LIMIT",
					Message:   "route branch targets self (recursion) but agent has no max_recursion set",
					AgentName: agentName, StepIndex: i,
				})
			}
			if target == agentName && maxRecursion > 100 {
				results = append(results, ValidationResult{
					Level: LevelWarning, Code: "EXCESSIVE_RECURSION",
					Message:   fmt.Sprintf("max_recursion=%d is very high", maxRecursion),
					AgentName: agentName, StepIndex: i,
				})
			}
		}
	}
	return results
}

// checkCyclicDeps 检测循环依赖（DFS）
func (v *Validator) checkCyclicDeps(
	agentName string,
	steps []Step,
	lookup AgentLookup,
) []ValidationResult {
	var results []ValidationResult

	// 构建当前 agent 的直接依赖
	deps := v.collectDeps(steps, agentName)
	if len(deps) == 0 {
		return results
	}

	// DFS 检测 A→B→...→A 循环
	visited := map[string]bool{agentName: true}
	for dep := range deps {
		if v.hasCycle(dep, agentName, visited, lookup) {
			results = append(results, ValidationResult{
				Level: LevelError, Code: "CYCLIC_DEPENDENCY",
				Message:   fmt.Sprintf("cyclic dependency detected: %s -> %s -> ... -> %s", agentName, dep, agentName),
				AgentName: agentName, StepIndex: -1,
			})
		}
	}
	return results
}

// collectDeps 从步骤中收集直接依赖的 agent 名称（排除自引用）
func (v *Validator) collectDeps(steps []Step, selfName string) map[string]bool {
	deps := map[string]bool{}
	for _, step := range steps {
		switch step.Type {
		case StepAgentRef:
			if step.Agent != selfName {
				deps[step.Agent] = true
			}
		case StepRoute:
			for _, target := range step.Branches {
				if target != selfName && target != RouteEndMarker {
					deps[target] = true
				}
			}
		}
	}
	return deps
}

// hasCycle DFS 检查从 current 出发是否能回到 origin
func (v *Validator) hasCycle(
	current, origin string,
	visited map[string]bool,
	lookup AgentLookup,
) bool {
	if current == origin {
		return true
	}
	if visited[current] {
		return false
	}
	visited[current] = true

	steps, exists := lookup(current)
	if !exists || len(steps) == 0 {
		return false
	}

	deps := v.collectDeps(steps, current)
	for dep := range deps {
		if v.hasCycle(dep, origin, visited, lookup) {
			return true
		}
	}
	return false
}
