package copilot

import (
	"context"
	"strings"

	"mote/pkg/logger"
)

// ToolBridge 负责将 mote 工具桥接到 ACP。
// 只桥接 CLI 不具备的工具，避免重复注册。
type ToolBridge struct {
	registry     ToolRegistryInterface
	excludeNames map[string]bool // CLI 已有的工具名，不桥接
}

// NewToolBridge 创建工具桥接器。
// 默认排除 CLI 已有的内置工具（shell, read_file, write_file, edit_file, list_dir）。
func NewToolBridge(registry ToolRegistryInterface) *ToolBridge {
	return &ToolBridge{
		registry: registry,
		excludeNames: map[string]bool{
			"shell":      true, // CLI 已有
			"read_file":  true,
			"write_file": true,
			"edit_file":  true,
			"list_dir":   true,
		},
	}
}

// NewToolBridgeWithExcludes 创建工具桥接器，使用自定义排除列表。
func NewToolBridgeWithExcludes(registry ToolRegistryInterface, excludeNames []string) *ToolBridge {
	excludeMap := make(map[string]bool, len(excludeNames))
	for _, name := range excludeNames {
		excludeMap[name] = true
	}
	return &ToolBridge{
		registry:     registry,
		excludeNames: excludeMap,
	}
}

// GetBridgeTools 返回需要桥接给 ACP 的工具定义。
// 工具名加 "mote_" 前缀避免与 CLI 内置工具冲突。
func (b *ToolBridge) GetBridgeTools() []ACPToolDef {
	if b == nil || b.registry == nil {
		return nil
	}

	tools := b.registry.ListToolInfo()
	var result []ACPToolDef
	for _, tool := range tools {
		if b.excludeNames[tool.Name] {
			continue
		}
		result = append(result, ACPToolDef{
			Name:        "mote_" + tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}

	logger.Debug().Int("bridged", len(result)).Int("excluded", len(b.excludeNames)).
		Msg("ToolBridge: prepared bridge tools")

	return result
}

// ExecuteTool 执行桥接工具调用。
// 自动去掉 "mote_" 前缀后调用 registry 执行。
func (b *ToolBridge) ExecuteTool(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	// 去掉 "mote_" 前缀
	moteName := strings.TrimPrefix(name, "mote_")

	logger.Debug().Str("tool", moteName).Msg("ToolBridge: executing bridged tool")

	execResult, err := b.registry.ExecuteTool(ctx, moteName, args)
	if err != nil {
		return ToolResult{
			TextResultForLLM: "Tool execution failed: " + err.Error(),
			ResultType:       "failure",
			Error:            err.Error(),
		}, nil // Return nil error — the failure is in the ToolResult
	}

	resultType := "success"
	if execResult.IsError {
		resultType = "failure"
	}

	return ToolResult{
		TextResultForLLM: execResult.Content,
		ResultType:       resultType,
	}, nil
}
