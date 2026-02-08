package channel

import (
	"context"
	"fmt"
	"sync"

	"mote/pkg/channel"
)

// Registry 渠道注册表
type Registry struct {
	plugins map[channel.ChannelType]channel.ChannelPlugin
	mu      sync.RWMutex
}

// NewRegistry 创建新的渠道注册表
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[channel.ChannelType]channel.ChannelPlugin),
	}
}

// Register 注册渠道插件
func (r *Registry) Register(plugin channel.ChannelPlugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[plugin.ID()] = plugin
}

// Get 获取指定渠道插件
func (r *Registry) Get(id channel.ChannelType) (channel.ChannelPlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

// All 获取所有渠道插件
func (r *Registry) All() []channel.ChannelPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]channel.ChannelPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}

// StartAll 启动所有渠道插件
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.plugins {
		if err := p.Start(ctx); err != nil {
			return fmt.Errorf("start channel %s: %w", p.ID(), err)
		}
	}
	return nil
}

// StopAll 停止所有渠道插件
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var lastErr error
	for _, p := range r.plugins {
		if err := p.Stop(ctx); err != nil {
			lastErr = fmt.Errorf("stop channel %s: %w", p.ID(), err)
		}
	}
	return lastErr
}

// Count 返回注册的插件数量
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}
