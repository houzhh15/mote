package hostapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"

	"mote/internal/jsvmerr"
)

// registerFS registers mote.fs API.
func registerFS(vm *goja.Runtime, mote *goja.Object, hctx *Context) error {
	fsObj := vm.NewObject()

	_ = fsObj.Set("read", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("path is required"))
		}

		path := call.Arguments[0].String()

		absPath, err := validatePath(path, hctx.Config.AllowedPaths)
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return goja.Null()
			}
			panic(vm.NewTypeError(fmt.Sprintf("read failed: %v", err)))
		}

		return vm.ToValue(string(content))
	})

	_ = fsObj.Set("write", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.NewTypeError("path and content are required"))
		}

		path := call.Arguments[0].String()
		content := call.Arguments[1].String()

		absPath, err := validatePath(path, hctx.Config.AllowedPaths)
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}

		// Check size limit
		if int64(len(content)) > hctx.Config.MaxWriteSize {
			panic(vm.NewTypeError(fmt.Sprintf("content exceeds max size of %d bytes", hctx.Config.MaxWriteSize)))
		}

		// Ensure directory exists
		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			panic(vm.NewTypeError(fmt.Sprintf("failed to create directory: %v", err)))
		}

		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			panic(vm.NewTypeError(fmt.Sprintf("write failed: %v", err)))
		}

		return goja.Undefined()
	})

	_ = fsObj.Set("exists", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("path is required"))
		}

		path := call.Arguments[0].String()

		absPath, err := validatePath(path, hctx.Config.AllowedPaths)
		if err != nil {
			// Path not allowed = doesn't exist from user's perspective
			return vm.ToValue(false)
		}

		_, err = os.Stat(absPath)
		return vm.ToValue(err == nil)
	})

	_ = fsObj.Set("list", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("path is required"))
		}

		path := call.Arguments[0].String()

		absPath, err := validatePath(path, hctx.Config.AllowedPaths)
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}

		entries, err := os.ReadDir(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return vm.ToValue([]string{})
			}
			panic(vm.NewTypeError(fmt.Sprintf("list failed: %v", err)))
		}

		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			names = append(names, name)
		}

		return vm.ToValue(names)
	})

	_ = mote.Set("fs", fsObj)
	return nil
}

// ValidatePathPublic is the exported version of validatePath for use by sandbox.
func ValidatePathPublic(path string, allowedPaths []string) (string, error) {
	return validatePath(path, allowedPaths)
}

// validatePath checks if the path is within allowed directories.
func validatePath(path string, allowedPaths []string) (string, error) {
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", &jsvmerr.PathNotAllowedError{Path: path}
		}
		path = filepath.Join(home, path[2:])
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", &jsvmerr.PathNotAllowedError{Path: path}
	}

	// Clean the path to prevent traversal
	absPath = filepath.Clean(absPath)

	// Resolve symlinks
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil && !os.IsNotExist(err) {
		// For write operations, parent must exist and be valid
		parentPath := filepath.Dir(absPath)
		realParent, parentErr := filepath.EvalSymlinks(parentPath)
		if parentErr != nil {
			return "", &jsvmerr.PathNotAllowedError{Path: path}
		}
		// Check parent is allowed, then use original absPath
		if !isPathAllowed(realParent, allowedPaths) {
			return "", &jsvmerr.PathNotAllowedError{Path: path}
		}
		return absPath, nil
	}
	if err == nil {
		absPath = realPath
	}

	// Check against allowlist
	if !isPathAllowed(absPath, allowedPaths) {
		return "", &jsvmerr.PathNotAllowedError{Path: path}
	}

	return absPath, nil
}

// isPathAllowed checks if path is under any allowed directory.
func isPathAllowed(path string, allowedPaths []string) bool {
	if len(allowedPaths) == 0 {
		return false // No paths allowed if list is empty
	}

	// Resolve the path to its real location (following symlinks)
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		// For non-existent files, use parent directory
		parentPath := filepath.Dir(path)
		realParent, parentErr := filepath.EvalSymlinks(parentPath)
		if parentErr == nil {
			realPath = filepath.Join(realParent, filepath.Base(path))
		} else {
			realPath = path
		}
	}

	for _, allowed := range allowedPaths {
		// Expand ~ in allowed paths
		if strings.HasPrefix(allowed, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			allowed = filepath.Join(home, allowed[2:])
		}

		allowed = filepath.Clean(allowed)

		// Also resolve symlinks in allowed path
		realAllowed, err := filepath.EvalSymlinks(allowed)
		if err != nil {
			realAllowed = allowed
		}

		// Check both original and resolved paths
		pathsToCheck := []string{path, realPath}
		allowedToCheck := []string{allowed, realAllowed}

		for _, p := range pathsToCheck {
			for _, a := range allowedToCheck {
				if strings.HasPrefix(p, a) {
					// Ensure it's a proper prefix
					if p == a || (len(p) > len(a) && p[len(a)] == filepath.Separator) {
						return true
					}
					if strings.HasSuffix(a, string(filepath.Separator)) {
						return true
					}
				}
			}
		}
	}

	return false
}
