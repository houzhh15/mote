package hostapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// registerHTTP registers mote.http API.
func registerHTTP(vm *goja.Runtime, mote *goja.Object, hctx *Context) error {
	httpObj := vm.NewObject()

	_ = httpObj.Set("get", func(call goja.FunctionCall) goja.Value {
		return doHTTPRequest(vm, hctx, "GET", call)
	})

	_ = httpObj.Set("post", func(call goja.FunctionCall) goja.Value {
		return doHTTPRequest(vm, hctx, "POST", call)
	})

	_ = httpObj.Set("put", func(call goja.FunctionCall) goja.Value {
		return doHTTPRequest(vm, hctx, "PUT", call)
	})

	_ = httpObj.Set("delete", func(call goja.FunctionCall) goja.Value {
		return doHTTPRequest(vm, hctx, "DELETE", call)
	})

	_ = mote.Set("http", httpObj)
	return nil
}

// doHTTPRequest performs an HTTP request and returns a Response object.
func doHTTPRequest(vm *goja.Runtime, hctx *Context, method string, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		panic(vm.NewTypeError("url is required"))
	}

	url := call.Arguments[0].String()

	// Check allowlist
	if !isURLAllowed(url, hctx.Config.HTTPAllowlist) {
		panic(vm.NewTypeError(fmt.Sprintf("URL not allowed: %s", url)))
	}

	// Parse body for POST/PUT
	var body io.Reader
	if method == "POST" || method == "PUT" {
		if len(call.Arguments) > 1 {
			bodyArg := call.Arguments[1]
			if bodyArg != nil && !goja.IsUndefined(bodyArg) && !goja.IsNull(bodyArg) {
				switch v := bodyArg.Export().(type) {
				case string:
					body = strings.NewReader(v)
				case map[string]interface{}, []interface{}:
					jsonBytes, err := json.Marshal(v)
					if err != nil {
						panic(vm.NewTypeError(fmt.Sprintf("failed to marshal body: %v", err)))
					}
					body = bytes.NewReader(jsonBytes)
				default:
					body = strings.NewReader(fmt.Sprintf("%v", v))
				}
			}
		}
	}

	// Parse options
	var headers map[string]string
	var timeout time.Duration = 30 * time.Second

	optIdx := 1
	if method == "POST" || method == "PUT" {
		optIdx = 2
	}
	if len(call.Arguments) > optIdx {
		optArg := call.Arguments[optIdx]
		if optArg != nil && !goja.IsUndefined(optArg) && !goja.IsNull(optArg) {
			if opts, ok := optArg.Export().(map[string]interface{}); ok {
				if h, ok := opts["headers"].(map[string]interface{}); ok {
					headers = make(map[string]string)
					for k, v := range h {
						headers[k] = fmt.Sprintf("%v", v)
					}
				}
				if t, ok := opts["timeout"].(float64); ok {
					timeout = time.Duration(t) * time.Millisecond
				}
			}
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(hctx.Ctx, method, url, body)
	if err != nil {
		panic(vm.NewTypeError(fmt.Sprintf("failed to create request: %v", err)))
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		panic(vm.NewTypeError(fmt.Sprintf("request failed: %v", err)))
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(vm.NewTypeError(fmt.Sprintf("failed to read response: %v", err)))
	}

	// Build response object
	return buildResponse(vm, resp, respBody)
}

// buildResponse creates a JS Response object.
func buildResponse(vm *goja.Runtime, resp *http.Response, body []byte) goja.Value {
	response := vm.NewObject()

	_ = response.Set("status", resp.StatusCode)

	// Body as string for convenient access
	_ = response.Set("body", string(body))

	// Headers as object
	headersObj := vm.NewObject()
	for k, v := range resp.Header {
		if len(v) > 0 {
			_ = headersObj.Set(strings.ToLower(k), v[0])
		}
	}
	_ = response.Set("headers", headersObj)

	// text() method
	_ = response.Set("text", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(string(body))
	})

	// json() method
	_ = response.Set("json", func(call goja.FunctionCall) goja.Value {
		var result interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			panic(vm.NewTypeError(fmt.Sprintf("failed to parse JSON: %v", err)))
		}
		return vm.ToValue(result)
	})

	return response
}

// isURLAllowed checks if the URL is in the allowlist.
func isURLAllowed(url string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true // Empty allowlist = allow all
	}

	for _, allowed := range allowlist {
		if strings.Contains(url, allowed) {
			return true
		}
	}
	return false
}
