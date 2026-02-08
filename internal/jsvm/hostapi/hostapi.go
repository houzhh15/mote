// Package hostapi provides JavaScript Host APIs for the jsvm sandbox.
package hostapi

import (
	"context"

	"github.com/dop251/goja"
	"github.com/rs/zerolog"

	"mote/internal/storage"
	"mote/internal/tools"
)

// Config holds configuration for Host APIs.
type Config struct {
	// AllowedPaths is the list of allowed file system paths.
	AllowedPaths []string
	// HTTPAllowlist is the list of allowed HTTP domains (empty = allow all).
	HTTPAllowlist []string
	// MaxWriteSize is the maximum file write size in bytes.
	MaxWriteSize int64
}

// DefaultConfig returns default Host API configuration.
func DefaultConfig() Config {
	return Config{
		AllowedPaths:  []string{"~/.mote/", "/tmp"},
		HTTPAllowlist: nil,
		MaxWriteSize:  10 * 1024 * 1024, // 10MB
	}
}

// Context holds the execution context for Host APIs.
type Context struct {
	Ctx         context.Context
	DB          *storage.DB
	Logger      zerolog.Logger
	ScriptName  string
	ExecutionID string
	Config      Config
}

// Register injects all Host APIs into the given goja.Runtime.
func Register(vm *goja.Runtime, hctx *Context) error {
	mote := vm.NewObject()

	// Register mote.http
	if err := registerHTTP(vm, mote, hctx); err != nil {
		return err
	}

	// Register mote.kv
	if err := registerKV(vm, mote, hctx); err != nil {
		return err
	}

	// Register mote.fs
	if err := registerFS(vm, mote, hctx); err != nil {
		return err
	}

	// Register mote.log
	if err := registerLog(vm, mote, hctx); err != nil {
		return err
	}

	// Register mote.context with session info from Go context
	if err := registerContext(vm, mote, hctx); err != nil {
		return err
	}

	// Set mote as global object
	_ = vm.Set("mote", mote)

	return nil
}

// registerContext injects mote.context with execution context info.
func registerContext(vm *goja.Runtime, mote *goja.Object, hctx *Context) error {
	ctxObj := vm.NewObject()

	// Extract session_id and agent_id from Go context
	if sessionID, ok := tools.SessionIDFromContext(hctx.Ctx); ok && sessionID != "" {
		_ = ctxObj.Set("session_id", sessionID)
	}
	if agentID, ok := tools.AgentIDFromContext(hctx.Ctx); ok && agentID != "" {
		_ = ctxObj.Set("agent_id", agentID)
	}

	// Set execution info
	_ = ctxObj.Set("script_name", hctx.ScriptName)
	_ = ctxObj.Set("execution_id", hctx.ExecutionID)

	_ = mote.Set("context", ctxObj)
	return nil
}

// Unregister removes Host APIs from the VM.
func Unregister(vm *goja.Runtime) {
	_ = vm.GlobalObject().Delete("mote")
}
