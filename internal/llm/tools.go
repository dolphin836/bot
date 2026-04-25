package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/dolphin836/bot/internal/memory"
)

// Compressor implements memory.Compressor using Claude to generate summaries
// and extract facts from conversation messages.
type Compressor struct {
	client anthropic.Client
	model  string
}

// NewCompressor creates a new Compressor backed by the Claude API.
func NewCompressor(apiKey string, model string) *Compressor {
	return &Compressor{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

// Summarize generates a concise 2-4 sentence summary of the given messages.
func (c *Compressor) Summarize(ctx context.Context, messages []memory.Message) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	conversation := formatMessages(messages)

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 512,
		System: []anthropic.TextBlockParam{
			{Text: "Summarize the following conversation in 2-4 sentences. Focus on the key topics discussed and any decisions or conclusions reached. Output only the summary, nothing else."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(conversation)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	return extractText(resp), nil
}

// ExtractFacts extracts key facts about the user from the given messages.
// Each fact is returned as a separate string in the slice.
func (c *Compressor) ExtractFacts(ctx context.Context, messages []memory.Message) ([]string, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	conversation := formatMessages(messages)

	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 512,
		System: []anthropic.TextBlockParam{
			{Text: "Extract key facts about the user from the following conversation. Output each fact on its own line, prefixed with \"- \". If there are no notable facts, output nothing."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(conversation)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("extract facts: %w", err)
	}

	text := extractText(resp)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	var facts []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			fact := strings.TrimPrefix(line, "- ")
			fact = strings.TrimSpace(fact)
			if fact != "" {
				facts = append(facts, fact)
			}
		}
	}

	if len(facts) == 0 {
		return nil, nil
	}

	return facts, nil
}

// formatMessages converts a slice of memory.Message into a readable conversation string.
func formatMessages(messages []memory.Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}
	return sb.String()
}

// extractText pulls out the text content from a Claude API response.
func extractText(msg *anthropic.Message) string {
	var sb strings.Builder
	for _, block := range msg.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			sb.WriteString(variant.Text)
		}
	}
	return sb.String()
}

// Verify at compile time that Compressor implements memory.Compressor.
var _ memory.Compressor = (*Compressor)(nil)
