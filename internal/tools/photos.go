package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dolphin836/bot/internal/memory"
)

type PhotosTool struct {
	store *memory.Store
}

func NewPhotosTool(store *memory.Store) *PhotosTool {
	return &PhotosTool{store: store}
}

func (p *PhotosTool) Name() string { return "search_photos" }

func (p *PhotosTool) Description() string {
	return "Search the user's personal photo album by keyword. Returns matching photos with descriptions. " +
		"Use this when the user asks about their photos, memories, past events, or wants to recall something they captured."
}

func (p *PhotosTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"keyword": {
				"type": "string",
				"description": "Search keyword to find photos (e.g. 'beach', 'birthday', 'cat', 'sunset')"
			}
		},
		"required": ["keyword"]
	}`)
}

type photosInput struct {
	Keyword string `json:"keyword"`
}

func (p *PhotosTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in photosInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	keyword := strings.TrimSpace(in.Keyword)
	if keyword == "" {
		return "Please provide a search keyword.", nil
	}

	photos, err := p.store.SearchPhotos(keyword)
	if err != nil {
		return "", fmt.Errorf("search photos: %w", err)
	}

	if len(photos) == 0 {
		return fmt.Sprintf("No photos found matching '%s'.", keyword), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d photo(s) matching '%s':\n\n", len(photos), keyword))
	for _, photo := range photos {
		sb.WriteString(fmt.Sprintf("- **%s** (%s, %s)\n  %s\n\n",
			photo.Filename,
			photo.FileType,
			photo.ModTime.Format("2006-01-02"),
			photo.Description,
		))
	}

	return sb.String(), nil
}
