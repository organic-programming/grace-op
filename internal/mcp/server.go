package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	sdkconnect "github.com/organic-programming/go-holons/pkg/connect"
	dopkg "github.com/organic-programming/grace-op/internal/do"
	inspectpkg "github.com/organic-programming/grace-op/internal/inspect"
	toolspkg "github.com/organic-programming/grace-op/internal/tools"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

const protocolVersion = "2025-06-18"

type Server struct {
	version string
	tools   []toolspkg.Definition
	prompts []promptDefinition

	toolIndex   map[string]toolBinding
	promptIndex map[string]promptDefinition
}

type toolBinding struct {
	slug       string
	definition toolspkg.Definition
	method     *inspectpkg.MethodBinding
	sequence   *inspectpkg.Sequence
}

type promptDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []promptArgument `json:"arguments,omitempty"`
	Text        string           `json:"-"`
}

type promptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NewServer loads one or more local holons and prepares MCP tool/prompt state.
func NewServer(slugs []string, version string) (*Server, error) {
	if len(slugs) == 0 {
		return nil, fmt.Errorf("at least one <slug> is required")
	}

	catalogs := make([]*inspectpkg.LocalCatalog, 0, len(slugs))
	for _, slug := range slugs {
		catalog, err := inspectpkg.LoadLocal(slug)
		if err != nil {
			return nil, err
		}
		catalogs = append(catalogs, catalog)
	}

	definitions := toolspkg.DefinitionsForCatalogs(catalogs)
	toolIndex := make(map[string]toolBinding, len(definitions))
	for _, catalog := range catalogs {
		for i := range catalog.Methods {
			method := catalog.Methods[i]
			name := method.ToolName(catalog.Slug)
			for _, definition := range definitions {
				if definition.Name != name {
					continue
				}
				toolIndex[name] = toolBinding{
					slug:       catalog.Slug,
					definition: definition,
					method:     &method,
				}
				break
			}
		}
		for i := range catalog.Document.Sequences {
			sequence := catalog.Document.Sequences[i]
			name := catalog.Slug + ".sequence." + sequence.Name
			for _, definition := range definitions {
				if definition.Name != name {
					continue
				}
				toolIndex[name] = toolBinding{
					slug:       catalog.Slug,
					definition: definition,
					sequence:   &sequence,
				}
				break
			}
		}
	}

	prompts := buildPromptDefinitions(catalogs)
	promptIndex := make(map[string]promptDefinition, len(prompts))
	for _, prompt := range prompts {
		promptIndex[prompt.Name] = prompt
	}

	return &Server{
		version:     version,
		tools:       definitions,
		prompts:     prompts,
		toolIndex:   toolIndex,
		promptIndex: promptIndex,
	}, nil
}

// ServeStdio runs the MCP server over newline-delimited JSON-RPC on stdio.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	writer := bufio.NewWriter(out)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return err
		}

		line = bytesTrimSpace(line)
		if len(line) > 0 {
			response, hasResponse := s.handle(ctx, line)
			if hasResponse {
				payload, marshalErr := json.Marshal(response)
				if marshalErr != nil {
					return marshalErr
				}
				if _, writeErr := writer.Write(payload); writeErr != nil {
					return writeErr
				}
				if writeErr := writer.WriteByte('\n'); writeErr != nil {
					return writeErr
				}
				if flushErr := writer.Flush(); flushErr != nil {
					return flushErr
				}
			}
		}

		if err == io.EOF {
			return nil
		}
	}
}

func (s *Server) handle(ctx context.Context, payload []byte) (rpcResponse, bool) {
	var request rpcRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		}, true
	}

	if len(request.ID) == 0 {
		if request.Method == "notifications/initialized" {
			return rpcResponse{}, false
		}
		return rpcResponse{}, false
	}

	response := rpcResponse{
		JSONRPC: "2.0",
		ID:      request.ID,
	}

	switch request.Method {
	case "initialize":
		response.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools":   map[string]any{},
				"prompts": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "op",
				"version": strings.TrimSpace(s.version),
			},
			"tools":   s.tools,
			"prompts": s.promptSummaries(),
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": s.tools}
	case "tools/call":
		result, err := s.handleToolCall(ctx, request.Params)
		if err != nil {
			response.Error = &rpcError{Code: -32602, Message: err.Error()}
			return response, true
		}
		response.Result = result
	case "prompts/list":
		response.Result = map[string]any{"prompts": s.promptSummaries()}
	case "prompts/get":
		result, err := s.handlePromptGet(request.Params)
		if err != nil {
			response.Error = &rpcError{Code: -32602, Message: err.Error()}
			return response, true
		}
		response.Result = result
	default:
		response.Error = &rpcError{Code: -32601, Message: "method not found"}
	}

	return response, true
}

func (s *Server) handleToolCall(ctx context.Context, params json.RawMessage) (map[string]any, error) {
	var request struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}

	binding, ok := s.toolIndex[request.Name]
	if !ok {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: fmt.Sprintf("unknown tool %q", request.Name),
			}},
			"isError": true,
		}, nil
	}

	if binding.sequence != nil {
		return s.handleSequenceCall(request.Name, binding, request.Arguments)
	}

	if binding.method == nil {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: fmt.Sprintf("tool %q is not bound to a callable target", request.Name),
			}},
			"isError": true,
		}, nil
	}

	if binding.method.Descriptor.IsStreamingClient() || binding.method.Descriptor.IsStreamingServer() {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: fmt.Sprintf("streaming RPC %q is not supported by op mcp", request.Name),
			}},
			"isError": true,
		}, nil
	}

	args, err := json.Marshal(request.Arguments)
	if err != nil || string(args) == "null" {
		args = []byte("{}")
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := sdkconnect.Connect(binding.slug)
	if err != nil {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: fmt.Sprintf("connect %s: %v", binding.slug, err),
			}},
			"isError": true,
		}, nil
	}
	defer func() { _ = sdkconnect.Disconnect(conn) }()

	inputMsg := dynamicpb.NewMessage(binding.method.Descriptor.Input())
	if len(args) > 0 && string(args) != "{}" {
		if err := protojson.Unmarshal(args, inputMsg); err != nil {
			return map[string]any{
				"content": []textContent{{
					Type: "text",
					Text: fmt.Sprintf("invalid tool arguments for %s: %v", request.Name, err),
				}},
				"isError": true,
			}, nil
		}
	}

	outputMsg := dynamicpb.NewMessage(binding.method.Descriptor.Output())
	if err := conn.Invoke(callCtx, binding.method.FullMethod(), inputMsg, outputMsg); err != nil {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: fmt.Sprintf("call %s failed: %v", request.Name, err),
			}},
			"isError": true,
		}, nil
	}

	outputJSON, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(outputMsg)
	if err != nil {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: fmt.Sprintf("marshal response for %s failed: %v", request.Name, err),
			}},
			"isError": true,
		}, nil
	}

	var structured map[string]any
	if err := json.Unmarshal(outputJSON, &structured); err != nil {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: string(outputJSON),
			}},
		}, nil
	}

	pretty, _ := json.MarshalIndent(structured, "", "  ")
	return map[string]any{
		"content": []textContent{{
			Type: "text",
			Text: string(pretty),
		}},
		"structuredContent": structured,
	}, nil
}

func (s *Server) handlePromptGet(params json.RawMessage) (map[string]any, error) {
	var request struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, fmt.Errorf("invalid prompts/get params: %w", err)
	}

	prompt, ok := s.promptIndex[request.Name]
	if !ok {
		return nil, fmt.Errorf("unknown prompt %q", request.Name)
	}

	return map[string]any{
		"description": prompt.Description,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": textContent{
					Type: "text",
					Text: prompt.Text,
				},
			},
		},
	}, nil
}

func (s *Server) promptSummaries() []promptDefinition {
	out := make([]promptDefinition, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		out = append(out, promptDefinition{
			Name:        prompt.Name,
			Description: prompt.Description,
			Arguments:   prompt.Arguments,
		})
	}
	return out
}

func buildPromptDefinitions(catalogs []*inspectpkg.LocalCatalog) []promptDefinition {
	out := make([]promptDefinition, 0)
	for _, catalog := range catalogs {
		if catalog == nil {
			continue
		}
		toolNames := make([]string, 0, len(catalog.Methods))
		for _, method := range catalog.Methods {
			toolNames = append(toolNames, method.ToolName(catalog.Slug))
		}
		for _, sequence := range catalog.Document.Sequences {
			toolNames = append(toolNames, catalog.Slug+".sequence."+sequence.Name)
		}
		sort.Strings(toolNames)

		for _, skill := range catalog.Document.Skills {
			var text strings.Builder
			fmt.Fprintf(&text, "Holon: %s\n", catalog.Slug)
			if strings.TrimSpace(skill.Description) != "" {
				fmt.Fprintf(&text, "Goal: %s\n", strings.TrimSpace(skill.Description))
			}
			if strings.TrimSpace(skill.When) != "" {
				fmt.Fprintf(&text, "When: %s\n", strings.TrimSpace(skill.When))
			}
			if len(skill.Steps) > 0 {
				text.WriteString("Steps:\n")
				for i, step := range skill.Steps {
					fmt.Fprintf(&text, "%d. %s\n", i+1, strings.TrimSpace(step))
				}
			}
			if len(toolNames) > 0 {
				text.WriteString("Available tools:\n")
				for _, name := range toolNames {
					fmt.Fprintf(&text, "- %s\n", name)
				}
			}

			out = append(out, promptDefinition{
				Name:        catalog.Slug + "." + skill.Name,
				Description: skill.Description,
				Text:        strings.TrimSpace(text.String()),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func bytesTrimSpace(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}

func (s *Server) handleSequenceCall(name string, binding toolBinding, args map[string]any) (map[string]any, error) {
	params := make(map[string]string, len(args))
	for key, value := range args {
		params[key] = stringifyArgument(value)
	}

	result, err := dopkg.Run(binding.slug, binding.sequence.Name, dopkg.Options{
		Params: params,
	})

	payload := any(result)
	isError := false
	if err != nil {
		isError = true
		payload = struct {
			*dopkg.Result
			Error string `json:"error"`
		}{
			Result: result,
			Error:  err.Error(),
		}
	}

	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: fmt.Sprintf("marshal response for %s failed: %v", name, marshalErr),
			}},
			"isError": true,
		}, nil
	}

	var structured map[string]any
	if err := json.Unmarshal(body, &structured); err != nil {
		return map[string]any{
			"content": []textContent{{
				Type: "text",
				Text: string(body),
			}},
			"isError": isError,
		}, nil
	}

	pretty, _ := json.MarshalIndent(structured, "", "  ")
	response := map[string]any{
		"content": []textContent{{
			Type: "text",
			Text: string(pretty),
		}},
		"structuredContent": structured,
	}
	if isError {
		response["isError"] = true
	}
	return response, nil
}

func stringifyArgument(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}
