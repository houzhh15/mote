package cli

import (
	"sync"

	"mote/internal/config"
	"mote/internal/storage"
	"mote/pkg/logger"

	"github.com/rs/zerolog"
)

// CLIContext CLI 上下文
type CLIContext struct {
	Config      *config.Config
	ConfigPath  string
	Logger      *zerolog.Logger
	storageOnce sync.Once
	storage     *storage.DB
	storagePath string
	StoragePath string // Exported for serve command
	Verbose     bool
	Quiet       bool
}

// NewCLIContext 创建 CLI 上下文
func NewCLIContext(cfg *config.Config, configPath string, log *zerolog.Logger, storagePath string, verbose, quiet bool) *CLIContext {
	return &CLIContext{
		Config:      cfg,
		ConfigPath:  configPath,
		Logger:      log,
		storagePath: storagePath,
		StoragePath: storagePath,
		Verbose:     verbose,
		Quiet:       quiet,
	}
}

// GetStorage 获取存储连接（懒加载）
func (c *CLIContext) GetStorage() (*storage.DB, error) {
	var err error
	c.storageOnce.Do(func() {
		c.storage, err = storage.Open(c.storagePath)
	})
	return c.storage, err
}

// Close 关闭资源
func (c *CLIContext) Close() error {
	if c.storage != nil {
		return c.storage.Close()
	}
	return nil
}

// Log 获取 Logger
func (c *CLIContext) Log() *zerolog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	log := logger.Get()
	return log
}
