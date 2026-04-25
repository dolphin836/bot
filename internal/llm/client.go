package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/dolphin836/bot/internal/memory"
	"github.com/dolphin836/bot/internal/tools"
)

// StreamCallback is called with the accumulated text so far on each text delta event.
type StreamCallback func(text string)

// Client wraps the Anthropic Claude API with streaming and tool use support.
type Client struct {
	client   anthropic.Client
	model    string
	registry *tools.Registry
}

// NewClient creates a new Claude API client.
func NewClient(apiKey string, model string, registry *tools.Registry) *Client {
	return &Client{
		client:   anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:    model,
		registry: registry,
	}
}

// BuildMessages converts a ConversationContext into a system prompt and message params
// suitable for the Anthropic API.
func BuildMessages(convCtx *memory.ConversationContext) (string, []anthropic.MessageParam) {
	// Build system prompt
	var sb strings.Builder
	sb.WriteString("You are a helpful personal assistant. Be concise and direct.")

	if len(convCtx.Facts) > 0 {
		sb.WriteString("\n\n## Things I know about the user\n")
		for _, f := range convCtx.Facts {
			sb.WriteString("- ")
			sb.WriteString(f.Content)
			sb.WriteString("\n")
		}
	}

	if len(convCtx.Summaries) > 0 {
		sb.WriteString("\n\n## Previous conversation summaries\n")
		for _, s := range convCtx.Summaries {
			sb.WriteString("- ")
			sb.WriteString(s.Content)
			sb.WriteString("\n")
		}
	}

	systemPrompt := sb.String()

	// Convert recent messages to API message params
	messages := make([]anthropic.MessageParam, 0, len(convCtx.RecentMessages))
	for i, msg := range convCtx.RecentMessages {
		isLast := i == len(convCtx.RecentMessages)-1

		switch msg.Role {
		case "user":
			if isLast && convCtx.ImageData != nil {
				messages = append(messages, anthropic.NewUserMessage(
					anthropic.NewImageBlockBase64(convCtx.ImageData.MediaType, convCtx.ImageData.Base64),
					anthropic.NewTextBlock(msg.Content),
				))
			} else {
				messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
			}
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
		default:
			// For tool results or other roles, treat as user message
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		}
	}

	return systemPrompt, messages
}

// BuildToolParams converts a tool registry into Anthropic API tool parameters.
func BuildToolParams(registry *tools.Registry) []anthropic.ToolUnionParam {
	toolList := registry.List()
	if len(toolList) == 0 {
		return nil
	}

	params := make([]anthropic.ToolUnionParam, 0, len(toolList))
	for _, t := range toolList {
		var schema anthropic.ToolInputSchemaParam
		if err := json.Unmarshal(t.InputSchema(), &schema); err != nil {
			// If schema parsing fails, skip this tool
			continue
		}

		params = append(params, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name(),
				Description: anthropic.String(t.Description()),
				InputSchema: schema,
			},
		})
	}

	return params
}

// SendStreaming sends a message to Claude with streaming and handles the tool use loop.
// It calls onText with the accumulated text on each text delta.
// Returns the final complete text response.
const maxToolIterations = 20

func (c *Client) SendStreaming(ctx context.Context, convCtx *memory.ConversationContext, onText StreamCallback) (string, error) {
	systemPrompt, messages := BuildMessages(convCtx)
	toolParams := BuildToolParams(c.registry)

	for iteration := 0; iteration < maxToolIterations; iteration++ {
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(c.model),
			MaxTokens: 4096,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  messages,
		}
		if len(toolParams) > 0 {
			params.Tools = toolParams
		}

		stream := c.client.Messages.NewStreaming(ctx, params)
		accumulated := anthropic.Message{}
		var fullText string

		for stream.Next() {
			event := stream.Current()
			if err := accumulated.Accumulate(event); err != nil {
				return "", fmt.Errorf("accumulate error: %w", err)
			}

			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch delta := ev.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					fullText += delta.Text
					if onText != nil {
						onText(fullText)
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			return "", fmt.Errorf("stream error: %w", err)
		}

		// Check for tool use blocks
		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range accumulated.Content {
			switch variant := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				inputJSON, err := json.Marshal(variant.Input)
				if err != nil {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						variant.ID, fmt.Sprintf("error marshaling input: %v", err), true,
					))
					continue
				}

				result, execErr := c.registry.Execute(ctx, variant.Name, inputJSON)
				if execErr != nil {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						variant.ID, fmt.Sprintf("error: %v", execErr), true,
					))
				} else {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						variant.ID, result, false,
					))
				}
			}
		}

		// If no tool use, return the text
		if len(toolResults) == 0 {
			return fullText, nil
		}

		// Append the assistant's response and tool results, then loop
		messages = append(messages, accumulated.ToParam())
		messages = append(messages, anthropic.NewUserMessage(toolResults...))

		// Reset fullText for next iteration
		fullText = ""
	}

	return "", fmt.Errorf("tool use loop exceeded %d iterations", maxToolIterations)
}
