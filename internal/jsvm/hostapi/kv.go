package hostapi

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dop251/goja"

	"mote/internal/storage"
)

const kvPrefix = "jsvm:"

// registerKV registers mote.kv API.
func registerKV(vm *goja.Runtime, mote *goja.Object, hctx *Context) error {
	kvObj := vm.NewObject()

	kvObj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("key is required"))
		}

		key := kvPrefix + call.Arguments[0].String()

		if hctx.DB == nil {
			return goja.Null()
		}

		value, err := hctx.DB.KVGet(key)
		if errors.Is(err, storage.ErrNotFound) {
			return goja.Null()
		}
		if err != nil {
			panic(vm.NewTypeError(fmt.Sprintf("kv get failed: %v", err)))
		}

		// Try to parse as JSON
		var result interface{}
		if err := json.Unmarshal([]byte(value), &result); err != nil {
			// Return as string if not valid JSON
			return vm.ToValue(value)
		}
		return vm.ToValue(result)
	})

	kvObj.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.NewTypeError("key and value are required"))
		}

		key := kvPrefix + call.Arguments[0].String()
		valueArg := call.Arguments[1].Export()

		// Serialize value to JSON
		var value string
		switch v := valueArg.(type) {
		case string:
			// Check if it's already valid JSON
			var js json.RawMessage
			if json.Unmarshal([]byte(v), &js) == nil {
				value = v
			} else {
				// Wrap string in quotes for JSON
				jsonBytes, _ := json.Marshal(v)
				value = string(jsonBytes)
			}
		default:
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				panic(vm.NewTypeError(fmt.Sprintf("failed to serialize value: %v", err)))
			}
			value = string(jsonBytes)
		}

		if hctx.DB == nil {
			return goja.Undefined()
		}

		if err := hctx.DB.KVSet(key, value, 0); err != nil {
			panic(vm.NewTypeError(fmt.Sprintf("kv set failed: %v", err)))
		}

		return goja.Undefined()
	})

	kvObj.Set("delete", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("key is required"))
		}

		key := kvPrefix + call.Arguments[0].String()

		if hctx.DB == nil {
			return goja.Undefined()
		}

		err := hctx.DB.KVDelete(key)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			panic(vm.NewTypeError(fmt.Sprintf("kv delete failed: %v", err)))
		}

		return goja.Undefined()
	})

	kvObj.Set("keys", func(call goja.FunctionCall) goja.Value {
		prefix := kvPrefix
		if len(call.Arguments) > 0 {
			prefix = kvPrefix + call.Arguments[0].String()
		}

		if hctx.DB == nil {
			return vm.ToValue([]string{})
		}

		result, err := hctx.DB.KVList(prefix)
		if err != nil {
			panic(vm.NewTypeError(fmt.Sprintf("kv list failed: %v", err)))
		}

		// Extract keys without prefix
		keys := make([]string, 0, len(result))
		for k := range result {
			// Remove jsvm: prefix for user-facing keys
			if len(k) > len(kvPrefix) {
				keys = append(keys, k[len(kvPrefix):])
			}
		}

		return vm.ToValue(keys)
	})

	mote.Set("kv", kvObj)
	return nil
}
