package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"

	"mote/internal/config"
	"mote/internal/provider/copilot"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// 敏感配置键（需要脱敏）
var sensitiveKeys = map[string]bool{
	"copilot.token":     true,
	"copilot.api_key":   true,
	"openai.api_key":    true,
	"anthropic.api_key": true,
}

// NewConfigCmd 创建 config 命令组
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  "Get, set, list, and edit configuration values",
	}

	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigListCmd())
	cmd.AddCommand(newConfigPathCmd())
	cmd.AddCommand(newConfigEditCmd())
	cmd.AddCommand(newConfigModelsCmd())
	cmd.AddCommand(newConfigScenariosCmd())

	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := config.Get(key)

			if value == nil {
				return fmt.Errorf("key not found: %s", key)
			}

			fmt.Println(value)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			if err := config.Set(key, value); err != nil {
				return fmt.Errorf("set config: %w", err)
			}

			fmt.Printf("Set %s = %s\n", key, value)
			return nil
		},
	}
}

func newConfigListCmd() *cobra.Command {
	var showAll bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configuration values",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings := viper.AllSettings()
			keys := flattenSettings("", settings)

			// 排序
			sort.Strings(keys)

			for _, key := range keys {
				value := viper.Get(key)

				// 脱敏处理
				if sensitiveKeys[key] && !showAll {
					if s, ok := value.(string); ok && s != "" {
						value = maskValue(s)
					}
				}

				fmt.Printf("%s = %v\n", key, value)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&showAll, "all", false, "show sensitive values")

	return cmd
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show configuration file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.DefaultConfigPath()
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		},
	}
}

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Edit configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.DefaultConfigPath()
			if err != nil {
				return err
			}

			// 获取编辑器
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = os.Getenv("VISUAL")
			}
			if editor == "" {
				editor = "vi"
			}

			// 打开编辑器
			c := exec.Command(editor, path)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr

			return c.Run()
		},
	}
}

// flattenSettings 将嵌套配置展平为点分隔的键列表
func flattenSettings(prefix string, settings map[string]any) []string {
	var keys []string

	for k, v := range settings {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if nested, ok := v.(map[string]any); ok {
			keys = append(keys, flattenSettings(key, nested)...)
		} else {
			keys = append(keys, key)
		}
	}

	return keys
}

// maskValue 脱敏处理
func maskValue(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}

// newConfigModelsCmd 创建 models 子命令
func newConfigModelsCmd() *cobra.Command {
	var (
		freeOnly    bool
		premiumOnly bool
		family      string
		jsonOutput  bool
		currentOnly bool
	)

	cmd := &cobra.Command{
		Use:   "models",
		Short: "List supported Copilot models",
		Long: `Display all supported Copilot models with their specifications.

Models are grouped by pricing:
  - Free models: Included in GitHub Copilot subscription
  - Premium models: Require premium request quota (multiplier shown)

Examples:
  mote config models              # List all models
  mote config models --free       # Only free models
  mote config models --family openai  # Only OpenAI models
  mote config models --json       # JSON output`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigModels(freeOnly, premiumOnly, family, jsonOutput, currentOnly)
		},
	}

	cmd.Flags().BoolVar(&freeOnly, "free", false, "Show only free models")
	cmd.Flags().BoolVar(&premiumOnly, "premium", false, "Show only premium models")
	cmd.Flags().StringVar(&family, "family", "", "Filter by family (openai|anthropic|google|xai)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&currentOnly, "current", false, "Show current model only")

	return cmd
}

// ModelsOutput represents the JSON output structure for models
type ModelsOutput struct {
	Models  []ModelInfo `json:"models"`
	Current string      `json:"current"`
	Default string      `json:"default"`
}

// ModelInfo represents a single model's information
type ModelInfo struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	Family         string `json:"family"`
	IsFree         bool   `json:"is_free"`
	Multiplier     int    `json:"multiplier"`
	ContextWindow  int    `json:"context_window"`
	MaxOutput      int    `json:"max_output"`
	SupportsVision bool   `json:"supports_vision"`
	SupportsTools  bool   `json:"supports_tools"`
	Description    string `json:"description"`
}

func runConfigModels(freeOnly, premiumOnly bool, family string, jsonOutput, currentOnly bool) error {
	// 获取当前模型
	currentModel := viper.GetString("copilot.model")
	if currentModel == "" {
		currentModel = copilot.DefaultModel
	}

	// 如果只显示当前模型
	if currentOnly {
		info := copilot.GetModelInfo(currentModel)
		if info == nil {
			fmt.Printf("Current: %s (unknown model)\n", currentModel)
			return nil
		}
		if jsonOutput {
			data, _ := json.MarshalIndent(ModelInfo{
				ID:             info.ID,
				DisplayName:    info.DisplayName,
				Family:         string(info.Family),
				IsFree:         info.IsFree,
				Multiplier:     info.Multiplier,
				ContextWindow:  info.ContextWindow,
				MaxOutput:      info.MaxOutput,
				SupportsVision: info.SupportsVision,
				SupportsTools:  info.SupportsTools,
				Description:    info.Description,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("Current Model: %s\n", info.ID)
			fmt.Printf("  Display Name: %s\n", info.DisplayName)
			fmt.Printf("  Family: %s\n", info.Family)
			fmt.Printf("  Free: %v\n", info.IsFree)
			if !info.IsFree {
				fmt.Printf("  Multiplier: %dx\n", info.Multiplier)
			}
			fmt.Printf("  Context Window: %s\n", formatContextWindow(info.ContextWindow))
			fmt.Printf("  Description: %s\n", info.Description)
		}
		return nil
	}

	// 过滤模型
	var models []ModelInfo
	for id, info := range copilot.SupportedModels {
		// 应用过滤器
		if freeOnly && !info.IsFree {
			continue
		}
		if premiumOnly && info.IsFree {
			continue
		}
		if family != "" && string(info.Family) != strings.ToLower(family) {
			continue
		}

		models = append(models, ModelInfo{
			ID:             id,
			DisplayName:    info.DisplayName,
			Family:         string(info.Family),
			IsFree:         info.IsFree,
			Multiplier:     info.Multiplier,
			ContextWindow:  info.ContextWindow,
			MaxOutput:      info.MaxOutput,
			SupportsVision: info.SupportsVision,
			SupportsTools:  info.SupportsTools,
			Description:    info.Description,
		})
	}

	// 排序：先按免费/付费分组，再按 ID 排序
	sort.Slice(models, func(i, j int) bool {
		if models[i].IsFree != models[j].IsFree {
			return models[i].IsFree // 免费的排前面
		}
		return models[i].ID < models[j].ID
	})

	// JSON 输出
	if jsonOutput {
		output := ModelsOutput{
			Models:  models,
			Current: currentModel,
			Default: copilot.DefaultModel,
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// 表格输出
	fmt.Println("Supported Copilot Models")
	fmt.Println("========================")
	fmt.Println()

	// 分离免费和付费模型
	var freeModels, premiumModels []ModelInfo
	for _, m := range models {
		if m.IsFree {
			freeModels = append(freeModels, m)
		} else {
			premiumModels = append(premiumModels, m)
		}
	}

	// 打印免费模型
	if len(freeModels) > 0 && !premiumOnly {
		fmt.Println("Free Models (included in subscription):")
		fmt.Printf("  %-20s %-22s %-12s %-10s %s\n", "ID", "Name", "Family", "Context", "Description")
		fmt.Println("  " + strings.Repeat("-", 85))
		for _, m := range freeModels {
			marker := ""
			if m.ID == currentModel {
				marker = " ←"
			}
			fmt.Printf("  %-20s %-22s %-12s %-10s %s%s\n",
				m.ID, m.DisplayName, m.Family,
				formatContextWindow(m.ContextWindow),
				truncate(m.Description, 25), marker)
		}
		fmt.Println()
	}

	// 打印付费模型
	if len(premiumModels) > 0 && !freeOnly {
		fmt.Println("Premium Models (requires premium requests):")
		fmt.Printf("  %-20s %-22s %-12s %-6s %-10s %s\n", "ID", "Name", "Family", "Mult.", "Context", "Description")
		fmt.Println("  " + strings.Repeat("-", 90))
		for _, m := range premiumModels {
			marker := ""
			if m.ID == currentModel {
				marker = " ←"
			}
			fmt.Printf("  %-20s %-22s %-12s %-6s %-10s %s%s\n",
				m.ID, m.DisplayName, m.Family,
				fmt.Sprintf("%dx", m.Multiplier),
				formatContextWindow(m.ContextWindow),
				truncate(m.Description, 20), marker)
		}
		fmt.Println()
	}

	// 显示当前模型
	isDefault := currentModel == copilot.DefaultModel
	defaultMark := ""
	if isDefault {
		defaultMark = " (default)"
	}
	fmt.Printf("Current: %s%s\n", currentModel, defaultMark)
	fmt.Println()
	fmt.Println("Usage: mote config set copilot.model <model-id>")

	return nil
}

// formatContextWindow 格式化上下文窗口大小
func formatContextWindow(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.0fM", float64(tokens)/1000000)
	}
	return fmt.Sprintf("%dK", tokens/1000)
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// newConfigScenariosCmd 创建 scenarios 子命令
func newConfigScenariosCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "scenarios [scenario] [model]",
		Short: "View or set default models for different scenarios",
		Long: `Manage default models for different scenarios (chat, cron, channel).

Each scenario can have its own default model. Sessions inherit the model
from their scenario unless explicitly overridden.

Scenarios:
  chat     Interactive chat sessions
  cron     Scheduled task executions
  channel  Channel-triggered workflows

Examples:
  mote config scenarios                    # List all scenario defaults
  mote config scenarios --json             # Output as JSON
  mote config scenarios chat               # Show chat scenario model
  mote config scenarios chat gpt-4.1       # Set chat default to gpt-4.1
  mote config scenarios cron claude-sonnet # Set cron default to claude-sonnet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runConfigScenariosGet(jsonOutput)
			} else if len(args) == 1 {
				return runConfigScenarioGet(args[0], jsonOutput)
			} else if len(args) == 2 {
				return runConfigScenarioSet(args[0], args[1])
			}
			return fmt.Errorf("too many arguments")
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

// runConfigScenariosGet 获取所有场景的默认模型
func runConfigScenariosGet(jsonOutput bool) error {
	addr := fmt.Sprintf("http://localhost:%d", viper.GetInt("server.port"))
	resp, err := http.Get(addr + "/api/v1/settings/models")
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}

	var models struct {
		Chat    string `json:"chat"`
		Cron    string `json:"cron"`
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(models, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Println("Scenario Default Models")
	fmt.Println("=======================")
	fmt.Printf("  %-12s %s\n", "Scenario", "Model")
	fmt.Println("  " + strings.Repeat("-", 40))
	fmt.Printf("  %-12s %s\n", "chat", models.Chat)
	fmt.Printf("  %-12s %s\n", "cron", models.Cron)
	fmt.Printf("  %-12s %s\n", "channel", models.Channel)
	fmt.Println()
	fmt.Println("Usage: mote config scenarios <scenario> <model-id>")

	return nil
}

// runConfigScenarioGet 获取单个场景的默认模型
func runConfigScenarioGet(scenario string, jsonOutput bool) error {
	validScenarios := map[string]bool{"chat": true, "cron": true, "channel": true}
	if !validScenarios[scenario] {
		return fmt.Errorf("invalid scenario: %s (valid: chat, cron, channel)", scenario)
	}

	addr := fmt.Sprintf("http://localhost:%d", viper.GetInt("server.port"))
	resp, err := http.Get(addr + "/api/v1/settings/models")
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}

	var models map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	model := models[scenario]

	if jsonOutput {
		data, _ := json.MarshalIndent(map[string]string{scenario: model}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("%s: %s\n", scenario, model)
	return nil
}

// runConfigScenarioSet 设置场景的默认模型
func runConfigScenarioSet(scenario, model string) error {
	validScenarios := map[string]bool{"chat": true, "cron": true, "channel": true}
	if !validScenarios[scenario] {
		return fmt.Errorf("invalid scenario: %s (valid: chat, cron, channel)", scenario)
	}

	// 验证模型是否有效
	if copilot.GetModelInfo(model) == nil {
		return fmt.Errorf("unknown model: %s (use 'mote config models' to list available models)", model)
	}

	addr := fmt.Sprintf("http://localhost:%d", viper.GetInt("server.port"))

	// 构建请求体
	body := map[string]string{scenario: model}
	data, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPut, addr+"/api/v1/settings/models", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("server error: %s", errResp.Error)
		}
		return fmt.Errorf("server returned %s", resp.Status)
	}

	fmt.Printf("Updated %s default model to: %s\n", scenario, model)
	return nil
}
