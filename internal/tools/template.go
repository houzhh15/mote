package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TargetType represents the target location for tools.
type TargetType string

const (
	// TargetUser indicates user-level tools directory (~/.mote/tools).
	TargetUser TargetType = "user"
	// TargetWorkspace indicates workspace-level tools directory (.mote/tools).
	TargetWorkspace TargetType = "workspace"
)

// RuntimeType represents the script runtime type for tool templates.
type RuntimeType string

const (
	RuntimeJavaScript RuntimeType = "javascript"
	RuntimePython     RuntimeType = "python"
	RuntimeBash       RuntimeType = "shell"
	RuntimePowerShell RuntimeType = "powershell"
)

// GetToolsDir returns the tools directory path for the given target.
func GetToolsDir(target TargetType) (string, error) {
	switch target {
	case TargetUser:
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		toolsDir := filepath.Join(homeDir, ".mote", "tools")
		// Ensure directory exists
		if err := os.MkdirAll(toolsDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create tools directory: %w", err)
		}
		return toolsDir, nil

	case TargetWorkspace:
		// Use current working directory for workspace-level tools
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
		toolsDir := filepath.Join(cwd, ".mote", "tools")
		// Ensure directory exists
		if err := os.MkdirAll(toolsDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create tools directory: %w", err)
		}
		return toolsDir, nil

	default:
		return "", fmt.Errorf("invalid target: %s (must be 'user' or 'workspace')", target)
	}
}

// CreateToolTemplate creates a new tool template at the specified target location.
// It creates the directory structure:
//
//	<tool-name>/
//	├── tool.json       # Tool metadata
//	└── handler.<ext>   # Handler script
//
// Returns the path to the created tool directory.
func CreateToolTemplate(name string, runtime RuntimeType, target TargetType) (string, error) {
	if name == "" {
		return "", fmt.Errorf("tool name cannot be empty")
	}

	// Sanitize the name for use as a directory
	dirName := strings.ToLower(strings.ReplaceAll(name, " ", "_"))

	// Determine base path based on target
	basePath, err := GetToolsDir(target)
	if err != nil {
		return "", fmt.Errorf("failed to get tools directory: %w", err)
	}

	toolDir := filepath.Join(basePath, dirName)

	// Check if directory already exists
	if _, err := os.Stat(toolDir); !os.IsNotExist(err) {
		return "", fmt.Errorf("tool directory already exists: %s", toolDir)
	}

	// Create tool directory
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create tool directory: %w", err)
	}

	// Determine handler filename based on runtime
	var handlerFile string
	switch runtime {
	case RuntimeJavaScript:
		handlerFile = "handler.js"
	case RuntimePython:
		handlerFile = "handler.py"
	case RuntimeBash:
		handlerFile = "handler.sh"
	case RuntimePowerShell:
		handlerFile = "handler.ps1"
	default:
		handlerFile = "handler.js"
		runtime = RuntimeJavaScript
	}

	// Create tool.json
	toolConfig := map[string]any{
		"name":        name,
		"description": fmt.Sprintf("A custom tool named %s", name),
		"runtime":     string(runtime),
		"handler":     handlerFile,
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "Input parameter",
				},
			},
			"required": []string{"input"},
		},
		"timeout":   30,
		"sandboxed": true,
	}

	toolJSON, err := json.MarshalIndent(toolConfig, "", "  ")
	if err != nil {
		os.RemoveAll(toolDir)
		return "", fmt.Errorf("failed to marshal tool config: %w", err)
	}

	toolJSONPath := filepath.Join(toolDir, "tool.json")
	if err := os.WriteFile(toolJSONPath, toolJSON, 0644); err != nil {
		os.RemoveAll(toolDir)
		return "", fmt.Errorf("failed to create tool.json: %w", err)
	}

	// Create handler file
	handlerContent := getHandlerTemplate(runtime, name)
	handlerPath := filepath.Join(toolDir, handlerFile)
	if err := os.WriteFile(handlerPath, []byte(handlerContent), 0755); err != nil {
		os.RemoveAll(toolDir)
		return "", fmt.Errorf("failed to create handler: %w", err)
	}

	return toolDir, nil
}

// getHandlerTemplate returns the template content for a handler file.
func getHandlerTemplate(runtime RuntimeType, toolName string) string {
	switch runtime {
	case RuntimeJavaScript:
		return fmt.Sprintf(`// JavaScript Tool Handler for %s
// Receives args object, returns result

module.exports = function(args) {
    // Get input parameter
    const input = args.input;
    
    // Process logic
    const result = "Processed: " + input;
    
    // Return result (string or object)
    return result;
};
`, toolName)

	case RuntimePython:
		return fmt.Sprintf(`#!/usr/bin/env python3
# Python Tool Handler for %s
# Reads JSON args from stdin, outputs result to stdout

import sys
import json

def handler(args):
    # Get input parameter
    input_text = args.get('input', '')
    
    # Process logic
    result = f'Processed: {input_text}'
    
    return result

if __name__ == '__main__':
    args = json.load(sys.stdin)
    result = handler(args)
    print(result)
`, toolName)

	case RuntimeBash:
		return fmt.Sprintf(`#!/bin/bash
# Shell Tool Handler for %s
# Args passed via ARGS environment variable (JSON format)

# Parse input (requires jq)
INPUT=$(echo "$ARGS" | jq -r '.input')

# Process logic
echo "Processed: $INPUT"
`, toolName)

	case RuntimePowerShell:
		return fmt.Sprintf(`# PowerShell Tool Handler for %s
# Args passed via ARGS environment variable (JSON format)

$args = $env:ARGS | ConvertFrom-Json
$input = $args.input

# Process logic
Write-Output "Processed: $input"
`, toolName)

	default:
		return "// Unknown runtime\n"
	}
}
