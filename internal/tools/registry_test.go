package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dolphin836/bot/internal/tools"
)

type echoTool struct{}

func (e *echoTool) Name() string        { return "echo" }
func (e *echoTool) Description() string { return "Echoes the input" }
func (e *echoTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`)
}
func (e *echoTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var req struct {
		Text string `json:"text"`
	}
	json.Unmarshal(input, &req)
	return req.Text, nil
}

func TestRegisterAndGet(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&echoTool{})

	tool, ok := r.Get("echo")
	if !ok {
		t.Fatal("expected to find registered tool 'echo'")
	}
	if tool.Name() != "echo" {
		t.Errorf("expected name 'echo', got %q", tool.Name())
	}
}

func TestGetMissing(t *testing.T) {
	r := tools.NewRegistry()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected tool 'nonexistent' to not be found")
	}
}

func TestExecute(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&echoTool{})

	result, err := r.Execute(context.Background(), "echo", json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected result 'hello', got %q", result)
	}
}

func TestExecuteMissingTool(t *testing.T) {
	r := tools.NewRegistry()

	_, err := r.Execute(context.Background(), "nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for nonexistent tool, got nil")
	}
}

func TestListTools(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&echoTool{})

	list := r.List()
	if len(list) != 1 {
		t.Errorf("expected 1 tool in list, got %d", len(list))
	}
}
