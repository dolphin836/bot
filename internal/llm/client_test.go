package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dolphin836/bot/internal/memory"
	"github.com/dolphin836/bot/internal/tools"
)

// testTool is a simple tool implementation for testing.
type testTool struct {
	name        string
	description string
	schema      json.RawMessage
}

func (t *testTool) Name() string                                                    { return t.name }
func (t *testTool) Description() string                                             { return t.description }
func (t *testTool) InputSchema() json.RawMessage                                    { return t.schema }
func (t *testTool) Execute(_ context.Context, _ json.RawMessage) (string, error) { return "ok", nil }

func TestBuildMessagesFromContext(t *testing.T) {
	convCtx := &memory.ConversationContext{
		Facts: []memory.Fact{
			{ID: 1, Content: "User likes Go", CreatedAt: time.Now()},
			{ID: 2, Content: "User lives in Tokyo", CreatedAt: time.Now()},
		},
		Summaries: []memory.Summary{
			{ID: 1, Content: "Discussed project setup and tooling preferences."},
		},
		RecentMessages: []memory.Message{
			{ID: 1, Role: "user", Content: "Hello", CreatedAt: time.Now()},
			{ID: 2, Role: "assistant", Content: "Hi there!", CreatedAt: time.Now()},
			{ID: 3, Role: "user", Content: "How are you?", CreatedAt: time.Now()},
		},
	}

	systemPrompt, messages := BuildMessages(convCtx)

	// Verify system prompt contains base persona
	if !strings.Contains(systemPrompt, "helpful personal assistant") {
		t.Error("system prompt should contain base persona")
	}

	// Verify system prompt contains facts
	if !strings.Contains(systemPrompt, "User likes Go") {
		t.Error("system prompt should contain facts")
	}
	if !strings.Contains(systemPrompt, "User lives in Tokyo") {
		t.Error("system prompt should contain all facts")
	}

	// Verify system prompt contains summaries
	if !strings.Contains(systemPrompt, "Discussed project setup") {
		t.Error("system prompt should contain summaries")
	}

	// Verify message count matches
	if len(messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(messages))
	}

	// Verify roles
	if messages[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %q", messages[0].Role)
	}
	if messages[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %q", messages[1].Role)
	}
	if messages[2].Role != "user" {
		t.Errorf("expected third message role 'user', got %q", messages[2].Role)
	}
}

func TestBuildMessagesEmptyContext(t *testing.T) {
	convCtx := &memory.ConversationContext{}

	systemPrompt, messages := BuildMessages(convCtx)

	if !strings.Contains(systemPrompt, "helpful personal assistant") {
		t.Error("system prompt should contain base persona even with empty context")
	}

	// No facts or summaries sections
	if strings.Contains(systemPrompt, "Things I know") {
		t.Error("system prompt should not contain facts section when no facts")
	}
	if strings.Contains(systemPrompt, "Previous conversation") {
		t.Error("system prompt should not contain summaries section when no summaries")
	}

	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}
}

func TestBuildMessagesWithImage(t *testing.T) {
	convCtx := &memory.ConversationContext{
		RecentMessages: []memory.Message{
			{ID: 1, Role: "user", Content: "What is this?", CreatedAt: time.Now()},
		},
		ImageData: &memory.ImageContent{
			Base64:    "aGVsbG8=",
			MediaType: "image/jpeg",
			Caption:   "A photo",
		},
	}

	_, messages := BuildMessages(convCtx)

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	// The last user message with image data should have 2 content blocks
	if len(messages[0].Content) != 2 {
		t.Errorf("expected 2 content blocks for image message, got %d", len(messages[0].Content))
	}
}

func TestBuildToolParams(t *testing.T) {
	registry := tools.NewRegistry()

	schema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)
	registry.Register(&testTool{
		name:        "search",
		description: "Search for something",
		schema:      schema,
	})
	registry.Register(&testTool{
		name:        "calculate",
		description: "Do math",
		schema:      json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`),
	})

	params := BuildToolParams(registry)

	if len(params) != 2 {
		t.Fatalf("expected 2 tool params, got %d", len(params))
	}

	// Verify each param has OfTool set
	for _, p := range params {
		if p.OfTool == nil {
			t.Error("expected OfTool to be set")
			continue
		}
		if p.OfTool.Name == "" {
			t.Error("expected tool name to be non-empty")
		}
	}
}

func TestBuildToolParamsEmpty(t *testing.T) {
	registry := tools.NewRegistry()
	params := BuildToolParams(registry)

	if params != nil {
		t.Errorf("expected nil params for empty registry, got %v", params)
	}
}
