package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"mote/internal/cli/defaults"
	"mote/internal/config"
	"mote/internal/storage"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// InitOptions init 命令选项
type InitOptions struct {
	Force bool
}

// NewInitCmd 创建 init 命令
func NewInitCmd() *cobra.Command {
	opts := &InitOptions{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize mote configuration",
		Long:  "Initialize mote configuration directory and files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunInit(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "overwrite existing configuration")

	return cmd
}

// RunInit 执行初始化
func RunInit(opts *InitOptions) error {
	// 获取配置目录
	configDir, err := config.DefaultConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}

	// 检查是否已存在
	configPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil && !opts.Force {
		return fmt.Errorf("configuration already exists at %s (use --force to overwrite)", configPath)
	}

	// 创建目录结构
	dirs := []string{
		configDir,
		filepath.Join(configDir, "logs"),
		filepath.Join(configDir, "ui"),
		filepath.Join(configDir, "tools"),
		filepath.Join(configDir, "skills"),
		filepath.Join(configDir, "prompts"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// 生成默认配置
	defaultConfig := map[string]any{
		"gateway": map[string]any{
			"port": 18788,
			"host": "127.0.0.1",
		},
		"provider": map[string]any{
			"default": "copilot", // 默认使用 copilot, 可选 "ollama"
		},
		"ollama": map[string]any{
			"endpoint":   "http://localhost:11434",
			"model":      "",
			"timeout":    "5m",
			"keep_alive": "5m",
		},
		"log": map[string]any{
			"level":  "info",
			"format": "console",
		},
		"memory": map[string]any{
			"enabled": true,
		},
		"cron": map[string]any{
			"enabled": true,
		},
		"mcp": map[string]any{
			"server": map[string]any{
				"enabled":   true,
				"transport": "stdio",
			},
		},
	}

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// 初始化数据库
	dataPath, err := config.DefaultDataPath()
	if err != nil {
		return fmt.Errorf("get data path: %w", err)
	}

	db, err := storage.Open(dataPath)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	db.Close()

	// Copy default skills
	if err := copyDefaultSkills(configDir, opts.Force); err != nil {
		fmt.Printf("Warning: failed to copy default skills: %v\n", err)
	}

	// Copy default prompts
	if err := copyDefaultPrompts(configDir, opts.Force); err != nil {
		fmt.Printf("Warning: failed to copy default prompts: %v\n", err)
	}

	fmt.Printf("Initialized mote at %s\n", configDir)
	fmt.Printf("  Config: %s\n", configPath)
	fmt.Printf("  Database: %s\n", dataPath)

	return nil
}

// copyDefaultPrompts copies embedded default prompts to the user's prompts directory.
func copyDefaultPrompts(configDir string, force bool) error {
	promptsDir := filepath.Join(configDir, "prompts")
	promptsFS := defaults.GetDefaultPromptsFS()

	return fs.WalkDir(promptsFS, "prompts", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == "prompts" {
			return nil
		}

		relPath, err := filepath.Rel("prompts", path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(promptsDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		if _, err := os.Stat(destPath); err == nil && !force {
			return nil
		}

		data, err := promptsFS.ReadFile(path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		return os.WriteFile(destPath, data, 0644)
	})
}

// copyDefaultSkills copies embedded default skills to the user's skills directory.
func copyDefaultSkills(configDir string, force bool) error {
	skillsDir := filepath.Join(configDir, "skills")
	defaultsFS := defaults.GetDefaultsFS()

	// Walk the embedded skills directory
	return fs.WalkDir(defaultsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root skills directory itself
		if path == "skills" {
			return nil
		}

		// Calculate the relative path and destination
		relPath, err := filepath.Rel("skills", path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(skillsDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Skip if file already exists and not forcing
		if _, err := os.Stat(destPath); err == nil && !force {
			return nil
		}

		// Read embedded file
		data, err := defaultsFS.ReadFile(path)
		if err != nil {
			return err
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// Write file
		return os.WriteFile(destPath, data, 0644)
	})
}
