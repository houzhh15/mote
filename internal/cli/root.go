package cli

import (
	"context"

	"mote/internal/config"
	"mote/pkg/logger"

	"github.com/spf13/cobra"
)

// GlobalFlags 全局标志
type GlobalFlags struct {
	ConfigPath string
	Verbose    bool
	Quiet      bool
}

var globalFlags GlobalFlags

// contextKey CLI 上下文键
//
//nolint:unused // Context key
type contextKey struct{}

// NewRootCmd 创建根命令
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "mote",
		Short: "Mote - Go AI Agent Runtime",
		Long: `Mote is a lightweight Go-based AI Agent runtime.
It provides session management, message storage, and CLI tools
for building and running AI agents.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 跳过 version 和 help 命令的初始化
			if cmd.Name() == "version" || cmd.Name() == "help" {
				return nil
			}

			// 确定配置路径
			configPath := globalFlags.ConfigPath
			if configPath == "" {
				var err error
				configPath, err = config.DefaultConfigPath()
				if err != nil {
					return err
				}
			}

			// 加载配置
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			// 初始化 Logger
			logLevel := cfg.Log.Level
			if globalFlags.Verbose {
				logLevel = "debug"
			}
			if globalFlags.Quiet {
				logLevel = "error"
			}

			if err := logger.Init(logger.LogConfig{
				Level:  logLevel,
				Format: cfg.Log.Format,
				File:   cfg.Log.File,
			}); err != nil {
				return err
			}

			// 确定存储路径
			storagePath := cfg.Storage.Path
			if storagePath == "" {
				storagePath, err = config.DefaultDataPath()
				if err != nil {
					return err
				}
			}

			// 创建 CLI 上下文
			log := logger.Get()
			cliCtx := NewCLIContext(cfg, configPath, log, storagePath, globalFlags.Verbose, globalFlags.Quiet)
			cmd.SetContext(context.WithValue(cmd.Context(), contextKey{}, cliCtx))

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			// 关闭资源
			cliCtx := GetCLIContext(cmd)
			if cliCtx != nil {
				return cliCtx.Close()
			}
			return nil
		},
	}

	// 添加全局标志
	rootCmd.PersistentFlags().StringVarP(&globalFlags.ConfigPath, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().BoolVarP(&globalFlags.Verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&globalFlags.Quiet, "quiet", "q", false, "quiet mode")

	// 添加子命令
	rootCmd.AddCommand(NewVersionCmd())
	rootCmd.AddCommand(NewInitCmd())
	rootCmd.AddCommand(NewConfigCmd())
	rootCmd.AddCommand(NewServeCmd())
	rootCmd.AddCommand(NewAuthCmd())
	rootCmd.AddCommand(NewChatCmd())
	rootCmd.AddCommand(NewSessionCmd())
	rootCmd.AddCommand(NewToolCmd())
	rootCmd.AddCommand(NewMemoryCmd())
	rootCmd.AddCommand(NewCronCmd())
	rootCmd.AddCommand(NewMCPCmd())
	rootCmd.AddCommand(NewDoctorCmd())
	rootCmd.AddCommand(NewModeCmd())
	rootCmd.AddCommand(NewUsageCmd())
	rootCmd.AddCommand(NewProviderCmd())
	rootCmd.AddCommand(NewSkillCmd())
	rootCmd.AddCommand(NewWorkspaceCmd())
	rootCmd.AddCommand(NewPromptCmd())

	return rootCmd
}

// GetCLIContext 从命令上下文获取 CLI 上下文
func GetCLIContext(cmd *cobra.Command) *CLIContext {
	ctx := cmd.Context()
	if ctx == nil {
		return nil
	}
	cliCtx, ok := ctx.Value(contextKey{}).(*CLIContext)
	if !ok {
		return nil
	}
	return cliCtx
}
