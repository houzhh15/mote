package config

import (
	"os"
	"path/filepath"
	"testing"
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
	if cfg.Gateway.Port != 8080 {
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
			name:     "Enabled 非空时直接返回",
			config:   ProviderConfig{Enabled: []string{"copilot", "ollama"}, Default: "copilot"},
			expected: []string{"copilot", "ollama"},
		},
		{
			name:     "Enabled 为空但 Default 非空时返回 Default",
			config:   ProviderConfig{Enabled: nil, Default: "ollama"},
			expected: []string{"ollama"},
		},
		{
			name:     "Enabled 和 Default 都为空时返回 copilot",
			config:   ProviderConfig{Enabled: nil, Default: ""},
			expected: []string{"copilot"},
		},
		{
			name:     "Enabled 为空切片时根据 Default 推断",
			config:   ProviderConfig{Enabled: []string{}, Default: "ollama"},
			expected: []string{"ollama"},
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
