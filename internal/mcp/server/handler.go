package server

import (
	"context"
	"encoding/json"
	"fmt"

	"mote/internal/mcp/protocol"
)

// HandlerFunc is a function that handles an MCP method.
type HandlerFunc func(ctx context.Context, params json.RawMessage) (any, error)

// MethodHandler handles MCP method calls.
type MethodHandler struct {
	server   *Server
	handlers map[string]HandlerFunc
}

// NewMethodHandler creates a new MethodHandler.
func NewMethodHandler(server *Server) *MethodHandler {
	h := &MethodHandler{
		server:   server,
		handlers: make(map[string]HandlerFunc),
	}
	h.registerHandlers()
	return h
}

// registerHandlers registers all built-in method handlers.
func (h *MethodHandler) registerHandlers() {
	h.handlers[protocol.MethodInitialize] = h.handleInitialize
	h.handlers[protocol.MethodToolsList] = h.handleToolsList
	h.handlers[protocol.MethodToolsCall] = h.handleToolsCall
	h.handlers[protocol.MethodPing] = h.handlePing
}

// HandleRequest handles a request message and returns a response.
func (h *MethodHandler) HandleRequest(ctx context.Context, req *protocol.Request) *protocol.Response {
	// Check if initialized (except for initialize method)
	if req.Method != protocol.MethodInitialize && !h.server.IsInitialized() {
		return protocol.NewErrorResponse(req.ID, protocol.NewNotInitializedError())
	}

	handler, ok := h.handlers[req.Method]
	if !ok {
		return protocol.NewErrorResponse(req.ID, protocol.NewMethodNotFoundError(req.Method))
	}

	result, err := handler(ctx, req.Params)
	if err != nil {
		// Check if it's already an RPC error
		if rpcErr, ok := err.(*protocol.RPCError); ok {
			return protocol.NewErrorResponse(req.ID, rpcErr)
		}
		return protocol.NewErrorResponse(req.ID, protocol.NewInternalError(err.Error()))
	}

	resp, err := protocol.NewResponse(req.ID, result)
	if err != nil {
		return protocol.NewErrorResponse(req.ID, protocol.NewInternalError(err.Error()))
	}
	return resp
}

// HandleNotification handles a notification message.
func (h *MethodHandler) HandleNotification(ctx context.Context, notif *protocol.Notification) {
	switch notif.Method {
	case protocol.MethodInitialized:
		// Client acknowledges initialization - nothing to do
	case protocol.MethodCancelled:
		// Handle cancellation if needed
	default:
		// Unknown notification - ignore
	}
}

// handleInitialize handles the initialize method.
func (h *MethodHandler) handleInitialize(ctx context.Context, params json.RawMessage) (any, error) {
	var initParams protocol.InitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		return nil, protocol.NewInvalidParamsError(err.Error())
	}

	// Validate protocol version
	if initParams.ProtocolVersion != protocol.ProtocolVersion {
		return nil, protocol.NewInvalidParamsError(
			fmt.Sprintf("unsupported protocol version: %s, expected: %s",
				initParams.ProtocolVersion, protocol.ProtocolVersion))
	}

	// Mark as initialized
	h.server.setInitialized(true)

	// Build capabilities
	capabilities := protocol.Capabilities{
		Tools: &protocol.ToolsCapability{
			ListChanged: false,
		},
	}

	// Build result
	result := protocol.InitializeResult{
		ProtocolVersion: protocol.ProtocolVersion,
		ServerInfo: protocol.ServerInfo{
			Name:    h.server.Name(),
			Version: h.server.Version(),
		},
		Capabilities: capabilities,
	}

	return result, nil
}

// handleToolsList handles the tools/list method.
func (h *MethodHandler) handleToolsList(ctx context.Context, params json.RawMessage) (any, error) {
	var listParams protocol.ListToolsParams
	if params != nil && len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &listParams); err != nil {
			return nil, protocol.NewInvalidParamsError(err.Error())
		}
	}

	tools := h.server.mapper.ListTools()
	result := protocol.ListToolsResult{
		Tools: tools,
	}

	return result, nil
}

// handleToolsCall handles the tools/call method.
func (h *MethodHandler) handleToolsCall(ctx context.Context, params json.RawMessage) (any, error) {
	var callParams protocol.CallToolParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, protocol.NewInvalidParamsError(err.Error())
	}

	if callParams.Name == "" {
		return nil, protocol.NewInvalidParamsError("tool name is required")
	}

	// Execute the tool
	result, err := h.server.mapper.Execute(ctx, callParams.Name, callParams.Arguments)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// handlePing handles the ping method.
func (h *MethodHandler) handlePing(ctx context.Context, params json.RawMessage) (any, error) {
	return struct{}{}, nil
}
