package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// NewChatCmd creates the chat command.
func NewChatCmd() *cobra.Command {
	var (
		sessionID string
		stream    bool
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "chat [message]",
		Short: "Send a message to the agent",
		Long: `Send a message to the Mote agent and get a response.

This command connects to a running Mote server (started with 'mote serve')
and sends a chat message to the agent.

If no message is provided as an argument, it will start an interactive chat session.`,
		Example: `  # Send a single message
  mote chat "Hello, how are you?"

  # Send a message with specific session
  mote chat --session abc123 "What did we discuss before?"

  # Stream the response
  mote chat --stream "Tell me a story"

  # Interactive chat
  mote chat`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read server URL from config if not specified via flag
			if serverURL == "" {
				cliCtx := GetCLIContext(cmd)
				if cliCtx != nil && cliCtx.Config != nil {
					host := cliCtx.Config.Gateway.Host
					port := cliCtx.Config.Gateway.Port
					if host == "" {
						host = "localhost"
					}
					if port == 0 {
						port = 18788
					}
					serverURL = fmt.Sprintf("http://%s:%d", host, port)
				} else {
					serverURL = "http://localhost:18788"
				}
			}
			return runChat(cmd, args, sessionID, stream, serverURL)
		},
	}

	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "session ID to continue conversation")
	cmd.Flags().BoolVar(&stream, "stream", false, "stream the response")
	cmd.Flags().StringVar(&serverURL, "url", "", "Mote server URL (reads from config if not specified)")

	return cmd
}

func runChat(cmd *cobra.Command, args []string, sessionID string, stream bool, serverURL string) error {
	// If no arguments, start interactive mode
	if len(args) == 0 {
		return runInteractiveChat(cmd, sessionID, serverURL)
	}

	// Join all arguments as the message
	message := strings.Join(args, " ")

	if stream {
		return sendStreamingMessage(serverURL, sessionID, message)
	}

	return sendSyncMessage(serverURL, sessionID, message)
}

func sendSyncMessage(serverURL, sessionID, message string) error {
	reqBody := map[string]interface{}{
		"message": message,
	}
	if sessionID != "" {
		reqBody["session_id"] = sessionID
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(
		serverURL+"/api/v1/chat",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to send request: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
		ToolCalls []struct {
			Name   string      `json:"name"`
			Result interface{} `json:"result"`
		} `json:"tool_calls,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Print response
	fmt.Println(chatResp.Message)

	// Print tool calls if any
	if len(chatResp.ToolCalls) > 0 {
		fmt.Println("\n[Tool Calls]")
		for _, tc := range chatResp.ToolCalls {
			fmt.Printf("- %s: %v\n", tc.Name, tc.Result)
		}
	}

	// Print session ID for reference
	if sessionID == "" {
		fmt.Printf("\n(Session ID: %s)\n", chatResp.SessionID)
	}

	return nil
}

func sendStreamingMessage(serverURL, sessionID, message string) error {
	reqBody := map[string]interface{}{
		"message": message,
	}
	if sessionID != "" {
		reqBody["session_id"] = sessionID
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(
		serverURL+"/api/v1/chat/stream",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to send request: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read SSE stream
	scanner := bufio.NewScanner(resp.Body)
	var currentSessionID string

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)
		switch eventType {
		case "content":
			if delta, ok := event["delta"].(string); ok {
				fmt.Print(delta)
			}
		case "tool_call":
			if toolCall, ok := event["tool_call"].(map[string]interface{}); ok {
				toolName, _ := toolCall["name"].(string)
				fmt.Printf("\n🔧 [Calling tool: %s]\n", toolName)
			}
		case "tool_result":
			if toolResult, ok := event["tool_result"].(map[string]interface{}); ok {
				toolName, _ := toolResult["tool_name"].(string)
				output, _ := toolResult["output"].(string)
				isError, _ := toolResult["is_error"].(bool)
				if isError {
					fmt.Printf("❌ [Tool %s failed]: %s\n", toolName, output)
				} else {
					// Truncate long outputs for display
					if len(output) > 500 {
						output = output[:500] + "...(truncated)"
					}
					fmt.Printf("✅ [Tool %s result]: %s\n", toolName, output)
				}
				fmt.Print("\n") // Add newline before next response
			}
		case "done":
			if sid, ok := event["session_id"].(string); ok {
				currentSessionID = sid
			}
			fmt.Println() // New line at end
		case "error":
			if errMsg, ok := event["message"].(string); ok {
				return fmt.Errorf("server error: %s", errMsg)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	// Print session ID for reference
	if sessionID == "" && currentSessionID != "" {
		fmt.Printf("\n(Session ID: %s)\n", currentSessionID)
	}

	return nil
}

func runInteractiveChat(cmd *cobra.Command, sessionID, serverURL string) error {
	fmt.Println("Mote Interactive Chat")
	fmt.Println("--------------------")
	fmt.Println("Type 'exit' or 'quit' to end the session")
	fmt.Println("Type 'clear' to start a new conversation")
	fmt.Println("")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("You: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println()
				return nil
			}
			return fmt.Errorf("failed to read input: %w", err)
		}

		message := strings.TrimSpace(input)

		// Handle special commands
		switch strings.ToLower(message) {
		case "exit", "quit":
			fmt.Println("Goodbye!")
			return nil
		case "clear":
			sessionID = ""
			fmt.Println("Starting new conversation...")
			continue
		case "":
			continue
		}

		// Send message
		fmt.Print("Agent: ")
		reqBody := map[string]interface{}{
			"message": message,
		}
		if sessionID != "" {
			reqBody["session_id"] = sessionID
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			fmt.Printf("\nError: %v\n\n", err)
			continue
		}

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Post(
			serverURL+"/api/v1/chat/stream",
			"application/json",
			bytes.NewBuffer(jsonData),
		)
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
			fmt.Println("Is the server running? Start it with: mote serve")
			continue
		}

		// Read SSE stream
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			eventType, _ := event["type"].(string)
			switch eventType {
			case "content":
				if delta, ok := event["delta"].(string); ok {
					fmt.Print(delta)
				}
			case "tool_call":
				if toolCall, ok := event["tool_call"].(map[string]interface{}); ok {
					toolName, _ := toolCall["name"].(string)
					fmt.Printf("\n🔧 [Calling tool: %s]\n", toolName)
				}
			case "tool_result":
				if toolResult, ok := event["tool_result"].(map[string]interface{}); ok {
					toolName, _ := toolResult["tool_name"].(string)
					output, _ := toolResult["output"].(string)
					isError, _ := toolResult["is_error"].(bool)
					if isError {
						fmt.Printf("❌ [Tool %s failed]: %s\n", toolName, output)
					} else {
						// Truncate long outputs for display
						if len(output) > 500 {
							output = output[:500] + "...(truncated)"
						}
						fmt.Printf("✅ [Tool %s result]: %s\n", toolName, output)
					}
					fmt.Print("Agent: ") // Resume agent response prefix
				}
			case "done":
				if sid, ok := event["session_id"].(string); ok {
					sessionID = sid // Update session ID for next message
				}
			case "error":
				if errMsg, ok := event["message"].(string); ok {
					fmt.Printf("\nError: %s", errMsg)
				}
			}
		}

		resp.Body.Close()
		fmt.Println()
	}
}
