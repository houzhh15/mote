package config

import (
	"os"
	"path/filepath"
	"testing"

	"mote/internal/cli/defaults"
)

func TestLoad_Defaults(t *testing.T) {
	Reset()
	defer Reset()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证默认值
	if cfg.Gateway.Port != 8080 {
		t.Errorf("gateway.port = %d, want 8080", cfg.Gateway.Port)
	}
	if cfg.Gateway.Host != "127.0.0.1" {
		t.Errorf("gateway.host = %q, want 127.0.0.1", cfg.Gateway.Host)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("log.level = %q, want info", cfg.Log.Level)
	}
	if cfg.Log.Format != "console" {
		t.Errorf("log.format = %q, want console", cfg.Log.Format)
	}
	if !cfg.Memory.Enabled {
		t.Error("memory.enabled = false, want true")
	}
	if !cfg.Cron.Enabled {
		t.Error("cron.enabled = false, want true")
	}
	if !cfg.MCP.Server.Enabled {
		t.Error("mcp.server.enabled = false, want true")
	}
	if cfg.MCP.Server.Transport != "stdio" {
		t.Errorf("mcp.server.transport = %q, want stdio", cfg.MCP.Server.Transport)
	}
}

func TestLoad_FromFile(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// 创建配置文件
	content := `
gateway:
  port: 9000
  host: "0.0.0.0"
log:
  level: debug
  format: json
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证文件中的值覆盖了默认值
	if cfg.Gateway.Port != 9000 {
		t.Errorf("gateway.port = %d, want 9000", cfg.Gateway.Port)
	}
	if cfg.Gateway.Host != "0.0.0.0" {
		t.Errorf("gateway.host = %q, want 0.0.0.0", cfg.Gateway.Host)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log.level = %q, want debug", cfg.Log.Level)
	}

	// 验证未在文件中指定的值使用默认值
	if !cfg.Memory.Enabled {
		t.Error("memory.enabled should use default value true")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	Reset()
	defer Reset()

	// 设置环境变量
	t.Setenv("MOTE_GATEWAY_PORT", "7777")
	t.Setenv("MOTE_LOG_LEVEL", "warn")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证环境变量覆盖了默认值
	if cfg.Gateway.Port != 7777 {
		t.Errorf("gateway.port = %d, want 7777", cfg.Gateway.Port)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("log.level = %q, want warn", cfg.Log.Level)
	}
}

func TestLoad_Priority(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// 创建配置文件设置 port=9000
	content := `
gateway:
  port: 9000
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// 设置环境变量覆盖 port=7777
	t.Setenv("MOTE_GATEWAY_PORT", "7777")

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证环境变量优先级高于配置文件
	if cfg.Gateway.Port != 7777 {
		t.Errorf("ENV should override file: gateway.port = %d, want 7777", cfg.Gateway.Port)
	}
}

func TestSetAndSave(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// 先加载以设置配置路径
	_, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 设置值
	if err := Set("gateway.port", 6666); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// 验证值已更新
	if GetInt("gateway.port") != 6666 {
		t.Errorf("gateway.port = %d, want 6666", GetInt("gateway.port"))
	}

	// 验证文件已写入
	Reset()
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if cfg.Gateway.Port != 6666 {
		t.Errorf("Persisted gateway.port = %d, want 6666", cfg.Gateway.Port)
	}
}

func TestGet_Functions(t *testing.T) {
	Reset()
	defer Reset()

	_, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 测试不同类型的 Get 函数
	if GetString("gateway.host") != "127.0.0.1" {
		t.Errorf("GetString failed")
	}
	if GetInt("gateway.port") != 8080 {
		t.Errorf("GetInt failed")
	}
	if !GetBool("memory.enabled") {
		t.Errorf("GetBool failed")
	}

	val := Get("gateway.port")
	if val == nil {
		t.Errorf("Get returned nil")
	}
}

func TestGetConfig(t *testing.T) {
	Reset()
	defer Reset()

	// 加载前应该返回 nil
	if GetConfig() != nil {
		t.Error("GetConfig should return nil before Load")
	}

	_, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cfg := GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig returned nil after Load")
	}
	if cfg.Gateway.Port != 8080 { //nolint:staticcheck // SA5011: Check above ensures non-nil
		t.Errorf("gateway.port = %d, want 8080", cfg.Gateway.Port)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// 创建无效的 YAML 文件
	content := `
gateway:
  port: [invalid
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	_, err := Load(configFile)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	Reset()
	defer Reset()

	// 加载不存在的文件应该不报错，使用默认值
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load should not fail for nonexistent file: %v", err)
	}

	// 应该使用默认值
	if cfg.Gateway.Port != 8080 {
		t.Errorf("gateway.port = %d, want default 8080", cfg.Gateway.Port)
	}
}

func TestSave_WithoutPath(t *testing.T) {
	Reset()
	defer Reset()

	// 不设置路径直接保存应该返回错误
	_, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err = Save()
	if err == nil {
		t.Error("Save should fail without config path")
	}
}

func TestProviderConfig_GetEnabledProviders(t *testing.T) {
	tests := []struct {
		name     string
		config   ProviderConfig
		expected []string
	}{
		{
			name:     "Enabled 包含 copilot 时自动替换为 copilot-acp",
			config:   ProviderConfig{Enabled: []string{"copilot", "ollama"}, Default: "copilot"},
			expected: []string{"copilot-acp", "ollama"},
		},
		{
			name:     "Enabled 仅有 copilot-acp 时直接返回",
			config:   ProviderConfig{Enabled: []string{"copilot-acp"}, Default: "copilot-acp"},
			expected: []string{"copilot-acp"},
		},
		{
			name:     "Enabled 同时包含 copilot 和 copilot-acp 时去掉 copilot",
			config:   ProviderConfig{Enabled: []string{"copilot", "copilot-acp", "ollama"}, Default: "copilot-acp"},
			expected: []string{"copilot-acp", "ollama"},
		},
		{
			name:     "Enabled 为空但 Default 非空时返回 Default",
			config:   ProviderConfig{Enabled: nil, Default: "ollama"},
			expected: []string{"ollama"},
		},
		{
			name:     "Enabled 和 Default 都为空时返回 copilot-acp",
			config:   ProviderConfig{Enabled: nil, Default: ""},
			expected: []string{"copilot-acp"},
		},
		{
			name:     "Enabled 为空切片时根据 Default 推断",
			config:   ProviderConfig{Enabled: []string{}, Default: "ollama"},
			expected: []string{"ollama"},
		},
		{
			name:     "Default 为 copilot 时迁移为 copilot-acp",
			config:   ProviderConfig{Enabled: nil, Default: "copilot"},
			expected: []string{"copilot-acp"},
		},
		{
			name:     "旧配置仅有 copilot 迁移为 copilot-acp",
			config:   ProviderConfig{Enabled: []string{"copilot"}, Default: "copilot"},
			expected: []string{"copilot-acp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetEnabledProviders()
			if len(result) != len(tt.expected) {
				t.Errorf("GetEnabledProviders() = %v, want %v", result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("GetEnabledProviders()[%d] = %v, want %v", i, v, tt.expected[i])
				}
			}
		})
	}
}

// ============ agents.yaml 独立配置测试 ============

func TestAgentsYAML_LoadOverride(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	agentsFile := filepath.Join(tmpDir, "agents.yaml")

	// config.yaml 中有 agent-a
	configContent := `
gateway:
  port: 8080
agents:
  agent-a:
    description: "from config.yaml"
    model: "model-a"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// agents.yaml 中有 agent-b（应覆盖 config.yaml 的 agents）
	agentsContent := `
agents:
  agent-b:
    description: "from agents.yaml"
    model: "model-b"
`
	if err := os.WriteFile(agentsFile, []byte(agentsContent), 0644); err != nil {
		t.Fatalf("Failed to write agents: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// agents.yaml 应完全覆盖 config.yaml 的 agents
	if _, exists := cfg.Agents["agent-a"]; exists {
		t.Error("agent-a should NOT exist (overridden by agents.yaml)")
	}
	if b, exists := cfg.Agents["agent-b"]; !exists {
		t.Error("agent-b should exist from agents.yaml")
	} else if b.Description != "from agents.yaml" {
		t.Errorf("agent-b.description = %q, want 'from agents.yaml'", b.Description)
	}
}

func TestAgentsYAML_FallbackToConfig(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// config.yaml 有 agents，agents.yaml 也会被自动安装（内嵌默认）
	// 但如果 agents.yaml 已存在，则不覆盖
	// 这里测试：预创建空 agents.yaml → 手动写入 agent-a → 确认加载
	configContent := `
gateway:
  port: 8080
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// 预创建 agents.yaml，包含自定义 agent
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	agentsContent := `
agents:
  agent-a:
    description: "custom agent"
`
	if err := os.WriteFile(agentsFile, []byte(agentsContent), 0644); err != nil {
		t.Fatalf("Failed to write agents: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// agents.yaml 已存在，不会安装默认版本，使用自定义 agent
	if a, exists := cfg.Agents["agent-a"]; !exists {
		t.Error("agent-a should exist from agents.yaml")
	} else if a.Description != "custom agent" {
		t.Errorf("agent-a.description = %q, want 'custom agent'", a.Description)
	}
}

func TestAgentsYAML_CRUDWritesToAgentsFile(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	agentsFile := filepath.Join(tmpDir, "agents.yaml")

	configContent := `
gateway:
  port: 8080
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// 预创建 agents.yaml（防止安装内嵌默认版本）
	agentsContent := `
agents:
  existing:
    description: "original"
`
	if err := os.WriteFile(agentsFile, []byte(agentsContent), 0644); err != nil {
		t.Fatalf("Failed to write agents: %v", err)
	}

	if _, err := Load(configFile); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// AddAgent 应写入 agents.yaml
	if err := AddAgent("new-agent", AgentConfig{Description: "new one"}); err != nil {
		t.Fatalf("AddAgent failed: %v", err)
	}

	data, err := os.ReadFile(agentsFile)
	if err != nil {
		t.Fatalf("agents.yaml should exist after AddAgent: %v", err)
	}

	content := string(data)
	if !contains(content, "new-agent") {
		t.Error("agents.yaml should contain new-agent")
	}
	if !contains(content, "existing") {
		t.Error("agents.yaml should also contain existing agent")
	}

	// UpdateAgent
	if err := UpdateAgent("existing", AgentConfig{Description: "updated"}); err != nil {
		t.Fatalf("UpdateAgent failed: %v", err)
	}

	data, err = os.ReadFile(agentsFile)
	if err != nil {
		t.Fatalf("Failed to read agents.yaml: %v", err)
	}
	if !contains(string(data), "updated") {
		t.Error("agents.yaml should contain updated description")
	}

	// RemoveAgent
	if err := RemoveAgent("new-agent"); err != nil {
		t.Fatalf("RemoveAgent failed: %v", err)
	}

	data, err = os.ReadFile(agentsFile)
	if err != nil {
		t.Fatalf("Failed to read agents.yaml: %v", err)
	}
	if contains(string(data), "new-agent") {
		t.Error("agents.yaml should NOT contain removed agent")
	}
}

func TestAgentsYAML_DefaultInstallation(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// 只有 config.yaml，没有 agents.yaml → 自动安装内嵌默认版本
	configContent := `
gateway:
  port: 8080
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 应自动安装内嵌默认 agents.yaml
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if _, err := os.Stat(agentsFile); os.IsNotExist(err) {
		t.Fatal("agents.yaml should be auto-installed from embedded default")
	}

	// 应加载内嵌的默认 agents（包含话题讨论等）
	if len(cfg.Agents) == 0 {
		t.Error("agents should be loaded from embedded default agents.yaml")
	}
	if _, exists := cfg.Agents["话题讨论"]; !exists {
		t.Error("话题讨论 agent should exist in default agents")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && len(substr) > 0 &&
		(s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ============ agents/ 目录加载测试 ============

func TestAgentsDir_MultiAgentFormat(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 预创建 agents.yaml（防止安装内嵌默认版本）
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents:\n  base-agent:\n    description: \"from agents.yaml\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 创建 agents/ 目录，包含标准格式文件
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	teamFile := filepath.Join(agentsDir, "team.yaml")
	teamContent := `agents:
  researcher:
    description: "research specialist"
    model: "gpt-4o"
  coder:
    description: "coding specialist"
    model: "claude-sonnet"
`
	if err := os.WriteFile(teamFile, []byte(teamContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// agents.yaml 中的 agent 应存在
	if _, exists := cfg.Agents["base-agent"]; !exists {
		t.Error("base-agent should exist from agents.yaml")
	}
	// agents/ 目录中的 agents 应存在
	if r, exists := cfg.Agents["researcher"]; !exists {
		t.Error("researcher should exist from agents/team.yaml")
	} else if r.Description != "research specialist" {
		t.Errorf("researcher.description = %q, want 'research specialist'", r.Description)
	}
	if c, exists := cfg.Agents["coder"]; !exists {
		t.Error("coder should exist from agents/team.yaml")
	} else if c.Model != "claude-sonnet" {
		t.Errorf("coder.model = %q, want 'claude-sonnet'", c.Model)
	}
}

func TestAgentsDir_SingleAgentFormat(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 预创建 agents.yaml
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 创建 agents/ 目录，包含单 agent 格式文件
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	singleFile := filepath.Join(agentsDir, "my-reviewer.yaml")
	singleContent := `description: "code review specialist"
model: "gpt-4o"
system_prompt: "You are a code reviewer."
tools:
  - read_file
  - grep
`
	if err := os.WriteFile(singleFile, []byte(singleContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 文件名（去掉扩展名）作为 agent 名称
	if r, exists := cfg.Agents["my-reviewer"]; !exists {
		t.Error("my-reviewer should exist from agents/my-reviewer.yaml")
	} else {
		if r.Description != "code review specialist" {
			t.Errorf("description = %q, want 'code review specialist'", r.Description)
		}
		if r.Model != "gpt-4o" {
			t.Errorf("model = %q, want 'gpt-4o'", r.Model)
		}
		if len(r.Tools) != 2 {
			t.Errorf("tools count = %d, want 2", len(r.Tools))
		}
	}
}

func TestAgentsDir_OverridesAgentsYAML(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// agents.yaml 中定义 same-agent
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	agentsContent := `agents:
  same-agent:
    description: "from agents.yaml"
    model: "old-model"
`
	if err := os.WriteFile(agentsFile, []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// agents/ 目录中也定义 same-agent（应覆盖 agents.yaml 中的）
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	overrideFile := filepath.Join(agentsDir, "override.yaml")
	overrideContent := `agents:
  same-agent:
    description: "from agents/ directory"
    model: "new-model"
`
	if err := os.WriteFile(overrideFile, []byte(overrideContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	agent, exists := cfg.Agents["same-agent"]
	if !exists {
		t.Fatal("same-agent should exist")
	}
	if agent.Description != "from agents/ directory" {
		t.Errorf("description = %q, want 'from agents/ directory'", agent.Description)
	}
	if agent.Model != "new-model" {
		t.Errorf("model = %q, want 'new-model'", agent.Model)
	}
}

func TestAgentsDir_NonExistentDirIsOK(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 预创建 agents.yaml
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents:\n  a1:\n    description: \"ok\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 不创建 agents/ 目录 → 应正常加载，没有错误
	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if _, exists := cfg.Agents["a1"]; !exists {
		t.Error("a1 should exist from agents.yaml even without agents/ dir")
	}
}

func TestAgentsDir_YMLExtension(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 预创建 agents.yaml
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 使用 .yml 扩展名
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	ymlFile := filepath.Join(agentsDir, "helper.yml")
	ymlContent := `description: "helper agent"
model: "gpt-4o-mini"
`
	if err := os.WriteFile(ymlFile, []byte(ymlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if h, exists := cfg.Agents["helper"]; !exists {
		t.Error("helper should exist from agents/helper.yml")
	} else if h.Model != "gpt-4o-mini" {
		t.Errorf("model = %q, want 'gpt-4o-mini'", h.Model)
	}
}

func TestAgentsDir_SkipsNonYAMLFiles(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 预创建 agents.yaml
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 非 yaml 文件应被跳过
	if err := os.WriteFile(filepath.Join(agentsDir, "notes.txt"), []byte("just notes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "README.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatal(err)
	}

	// 一个有效的 yaml 文件
	validFile := filepath.Join(agentsDir, "valid.yaml")
	if err := os.WriteFile(validFile, []byte("description: \"valid\"\nmodel: \"m\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if _, exists := cfg.Agents["valid"]; !exists {
		t.Error("valid agent should exist")
	}
	// notes.txt 和 README.md 不应被加载为 agent
	if _, exists := cfg.Agents["notes"]; exists {
		t.Error("notes.txt should not be loaded as agent")
	}
	if _, exists := cfg.Agents["README"]; exists {
		t.Error("README.md should not be loaded as agent")
	}
}

func TestAgentsDir_MultipleFiles(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 预创建 agents.yaml
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents:\n  base:\n    description: \"base\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 文件 1: 标准格式，多个 agents
	file1 := filepath.Join(agentsDir, "team-a.yaml")
	if err := os.WriteFile(file1, []byte("agents:\n  a1:\n    description: \"a1\"\n  a2:\n    description: \"a2\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 文件 2: 单 agent 格式
	file2 := filepath.Join(agentsDir, "solo.yaml")
	if err := os.WriteFile(file2, []byte("description: \"solo agent\"\nmodel: \"m\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 应有 4 个 agents: base + a1 + a2 + solo
	expected := []string{"base", "a1", "a2", "solo"}
	for _, name := range expected {
		if _, exists := cfg.Agents[name]; !exists {
			t.Errorf("agent %q should exist", name)
		}
	}
}

func TestReloadAgents_PicksUpNewFiles(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents:\n  original:\n    description: \"original\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	initialCount := len(cfg.Agents)
	if _, exists := cfg.Agents["original"]; !exists {
		t.Fatal("original agent should exist initially")
	}

	// 在 agents/ 目录中写入新文件（模拟 LLM 创建）
	newFile := filepath.Join(agentsDir, "new-agent.yaml")
	if err := os.WriteFile(newFile, []byte("description: \"new agent\"\nmodel: \"m\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 调用 ReloadAgents 应该加载到新文件
	count, err := ReloadAgents()
	if err != nil {
		t.Fatalf("ReloadAgents failed: %v", err)
	}
	if count != initialCount+1 {
		t.Errorf("expected %d agents after reload (initial %d + 1 new), got %d", initialCount+1, initialCount, count)
	}

	reloaded := GetConfig()
	if _, exists := reloaded.Agents["original"]; !exists {
		t.Error("original agent should still exist")
	}
	if _, exists := reloaded.Agents["new-agent"]; !exists {
		t.Error("new-agent should exist after reload")
	}
}

func TestReloadAgents_DirOverridesYAML(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents:\n  myagent:\n    description: \"from yaml\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(agentsDir, "myagent.yaml")
	if err := os.WriteFile(override, []byte("description: \"from dir\"\nmodel: \"m\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	_, err = ReloadAgents()
	if err != nil {
		t.Fatalf("ReloadAgents failed: %v", err)
	}

	reloaded := GetConfig()
	if reloaded.Agents["myagent"].Description != "from dir" {
		t.Errorf("expected description 'from dir', got %q", reloaded.Agents["myagent"].Description)
	}
}

func TestValidateAgentsDir_Mixed(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 有效文件：标准格式
	if err := os.WriteFile(filepath.Join(agentsDir, "good.yaml"),
		[]byte("agents:\n  a1:\n    description: \"a1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 有效文件：单 agent 格式
	if err := os.WriteFile(filepath.Join(agentsDir, "solo.yaml"),
		[]byte("description: \"solo\"\nmodel: \"m\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 无效文件：YAML 语法错误
	if err := os.WriteFile(filepath.Join(agentsDir, "bad.yaml"),
		[]byte(":\n  bad yaml [[\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 无效文件：不符合任何格式
	if err := os.WriteFile(filepath.Join(agentsDir, "unknown.yaml"),
		[]byte("foo: bar\nbaz: 123\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 非 YAML 文件，应被忽略
	if err := os.WriteFile(filepath.Join(agentsDir, "readme.md"),
		[]byte("# readme\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 需要 Load 来设置 agentsDirPath
	if _, err := Load(configFile); err != nil {
		t.Fatal(err)
	}

	// 删除 Load 过程中自动安装的默认 agent 文件，只测试手动创建的文件
	defaultFiles := defaults.GetDefaultAgentDirFiles()
	for name := range defaultFiles {
		os.Remove(filepath.Join(agentsDir, name))
	}

	summary := ValidateAgentsDir()

	if !summary.Exists {
		t.Error("expected Exists=true")
	}
	if summary.TotalFiles != 4 {
		t.Errorf("expected TotalFiles=4 (YAML only), got %d", summary.TotalFiles)
	}
	if summary.ValidFiles != 2 {
		t.Errorf("expected ValidFiles=2, got %d", summary.ValidFiles)
	}
	if summary.TotalAgents != 2 {
		t.Errorf("expected TotalAgents=2 (a1 + solo), got %d", summary.TotalAgents)
	}

	// 验证各结果
	resultMap := make(map[string]AgentsDirValidationResult)
	for _, r := range summary.Results {
		resultMap[r.File] = r
	}

	if r, ok := resultMap["good.yaml"]; !ok || !r.Valid || r.Format != "multi" {
		t.Errorf("good.yaml should be valid multi format, got %+v", r)
	}
	if r, ok := resultMap["solo.yaml"]; !ok || !r.Valid || r.Format != "single" {
		t.Errorf("solo.yaml should be valid single format, got %+v", r)
	}
	if r, ok := resultMap["bad.yaml"]; !ok || r.Valid || r.Error == "" {
		t.Errorf("bad.yaml should be invalid with error, got %+v", r)
	}
	if r, ok := resultMap["unknown.yaml"]; !ok || r.Valid || r.Error == "" {
		t.Errorf("unknown.yaml should be invalid with error, got %+v", r)
	}
}

func TestValidateAgentsDir_NonExistent(t *testing.T) {
	Reset()
	defer Reset()

	// 没有调用 Load，agentsDirPath 为空
	summary := ValidateAgentsDir()
	if summary.Exists {
		t.Error("expected Exists=false for empty dir path")
	}
	if summary.TotalFiles != 0 {
		t.Errorf("expected TotalFiles=0, got %d", summary.TotalFiles)
	}
}

// TestSaveAgents_SourceAwareWriteback 验证 saveAgents 将来自 agents/ 目录的 agent
// 写回源文件而不是写入 agents.yaml。
func TestSaveAgents_SourceAwareWriteback(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// agents.yaml 中有一个 agent
	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents:\n  yaml-agent:\n    description: \"from yaml\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// agents/ 目录中有一个多 agent 格式的文件
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	dirFile := filepath.Join(agentsDir, "team.yaml")
	if err := os.WriteFile(dirFile, []byte("agents:\n  dir-agent-a:\n    description: \"dir a\"\n  dir-agent-b:\n    description: \"dir b\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 更新来自 agents/ 目录的 agent
	if err := UpdateAgent("dir-agent-a", AgentConfig{Description: "dir a updated"}); err != nil {
		t.Fatalf("UpdateAgent failed: %v", err)
	}

	// 验证 agents.yaml 不包含来自 agents/ 目录的 agent
	data, err := os.ReadFile(agentsFile)
	if err != nil {
		t.Fatalf("Failed to read agents.yaml: %v", err)
	}
	content := string(data)
	if contains(content, "dir-agent-a") {
		t.Error("agents.yaml should NOT contain dir-agent-a (it belongs to agents/ dir)")
	}
	if contains(content, "dir-agent-b") {
		t.Error("agents.yaml should NOT contain dir-agent-b (it belongs to agents/ dir)")
	}
	if !contains(content, "yaml-agent") {
		t.Error("agents.yaml should still contain yaml-agent")
	}

	// 验证 agents/ 目录文件包含更新后的 agent
	dirData, err := os.ReadFile(dirFile)
	if err != nil {
		t.Fatalf("Failed to read dir file: %v", err)
	}
	dirContent := string(dirData)
	if !contains(dirContent, "dir-agent-a") {
		t.Error("dir file should contain dir-agent-a")
	}
	if !contains(dirContent, "dir a updated") {
		t.Error("dir file should contain updated description")
	}
	if !contains(dirContent, "dir-agent-b") {
		t.Error("dir file should contain dir-agent-b")
	}
}

// TestSaveAgents_SingleFormatWriteback 验证单 agent 格式文件的回写。
func TestSaveAgents_SingleFormatWriteback(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("gateway:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agentsFile := filepath.Join(tmpDir, "agents.yaml")
	if err := os.WriteFile(agentsFile, []byte("agents: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// agents/ 目录中有一个单 agent 格式的文件
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	singleFile := filepath.Join(agentsDir, "solo-agent.yaml")
	if err := os.WriteFile(singleFile, []byte("description: \"solo original\"\nmodel: \"gpt-4\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 更新来自单文件的 agent
	if err := UpdateAgent("solo-agent", AgentConfig{Description: "solo updated", Model: "gpt-4o"}); err != nil {
		t.Fatalf("UpdateAgent failed: %v", err)
	}

	// 验证 agents.yaml 不包含来自 agents/ 目录的 agent
	data, _ := os.ReadFile(agentsFile)
	if contains(string(data), "solo-agent") {
		t.Error("agents.yaml should NOT contain solo-agent")
	}

	// 验证 agents/ 目录文件被正确回写（单格式，不包含 agents: wrapper）
	dirData, err := os.ReadFile(singleFile)
	if err != nil {
		t.Fatalf("Failed to read single agent file: %v", err)
	}
	dirContent := string(dirData)
	if !contains(dirContent, "solo updated") {
		t.Error("single agent file should contain updated description")
	}
	// 单 agent 格式不应包含 "agents:" wrapper
	if contains(dirContent, "agents:") {
		t.Error("single agent file should NOT have agents: wrapper")
	}
}
