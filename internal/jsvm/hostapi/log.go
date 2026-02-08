package hostapi

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/rs/zerolog"
)

// registerLog registers mote.log API.
func registerLog(vm *goja.Runtime, mote *goja.Object, hctx *Context) error {
	logObj := vm.NewObject()

	logger := hctx.Logger.With().
		Str("script", hctx.ScriptName).
		Str("exec_id", hctx.ExecutionID).
		Logger()

	logObj.Set("debug", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Debug().Msg(msg)
		return goja.Undefined()
	})

	logObj.Set("info", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Info().Msg(msg)
		return goja.Undefined()
	})

	logObj.Set("warn", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Warn().Msg(msg)
		return goja.Undefined()
	})

	logObj.Set("error", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Error().Msg(msg)
		return goja.Undefined()
	})

	mote.Set("log", logObj)

	// Also register console.log for convenience
	registerConsole(vm, logger)

	return nil
}

// formatLogMessage formats log arguments into a single message string.
func formatLogMessage(args []goja.Value) string {
	if len(args) == 0 {
		return ""
	}

	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = formatValue(arg)
	}

	// Join with spaces like console.log
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += " " + parts[i]
	}
	return result
}

// formatValue converts a goja.Value to a string representation.
func formatValue(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}

	exported := v.Export()
	switch val := exported.(type) {
	case string:
		return val
	case map[string]interface{}, []interface{}:
		// Format objects/arrays as JSON-like
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// registerConsole adds a basic console object for compatibility.
func registerConsole(vm *goja.Runtime, logger zerolog.Logger) {
	console := vm.NewObject()

	console.Set("log", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Info().Msg(msg)
		return goja.Undefined()
	})

	console.Set("debug", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Debug().Msg(msg)
		return goja.Undefined()
	})

	console.Set("info", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Info().Msg(msg)
		return goja.Undefined()
	})

	console.Set("warn", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Warn().Msg(msg)
		return goja.Undefined()
	})

	console.Set("error", func(call goja.FunctionCall) goja.Value {
		msg := formatLogMessage(call.Arguments)
		logger.Error().Msg(msg)
		return goja.Undefined()
	})

	vm.Set("console", console)
}
