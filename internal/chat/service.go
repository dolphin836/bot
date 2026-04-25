package chat

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strings"

	"github.com/dolphin836/bot/internal/llm"
	"github.com/dolphin836/bot/internal/memory"
)

type Service struct {
	memMgr    *memory.Manager
	llmClient *llm.Client
}

func NewService(memMgr *memory.Manager, llmClient *llm.Client) *Service {
	return &Service{
		memMgr:    memMgr,
		llmClient: llmClient,
	}
}

func (s *Service) HandleMessage(ctx context.Context, content string, contentType string, onStream llm.StreamCallback) (string, error) {
	// For images, store only the caption
	storeContent := content
	if contentType == "image" {
		if parts := strings.SplitN(content, "|||", 2); len(parts) == 2 {
			storeContent = "[image] " + parts[1]
		}
	}

	if err := s.memMgr.Store().AddMessage(&memory.Message{
		Role:        "user",
		Content:     storeContent,
		ContentType: contentType,
	}); err != nil {
		return "", err
	}

	convCtx, err := s.memMgr.BuildContext(ctx)
	if err != nil {
		return "", err
	}

	// For image messages, inject image data into context
	if contentType == "image" {
		if parts := strings.SplitN(content, "|||", 2); len(parts) == 2 {
			if _, err := base64.StdEncoding.DecodeString(parts[0]); err == nil {
				// Update last message content to caption only
				if len(convCtx.RecentMessages) > 0 {
					convCtx.RecentMessages[len(convCtx.RecentMessages)-1].Content = parts[1]
				}
				convCtx.ImageData = &memory.ImageContent{
					Base64:    parts[0],
					MediaType: "image/jpeg",
					Caption:   parts[1],
				}
			}
		}
	}

	reply, err := s.llmClient.SendStreaming(ctx, convCtx, onStream)
	if err != nil {
		return "", err
	}

	if err := s.memMgr.Store().AddMessage(&memory.Message{
		Role:        "assistant",
		Content:     reply,
		ContentType: "text",
	}); err != nil {
		slog.Error("save_assistant_message", "error", err)
	}

	if err := s.memMgr.CompressIfNeeded(ctx); err != nil {
		slog.Error("compress", "error", err)
	}

	return reply, nil
}

func (s *Service) ClearAll() error {
	return s.memMgr.Store().ClearAll()
}

func (s *Service) GetFacts() ([]memory.Fact, error) {
	return s.memMgr.Store().GetAllFacts()
}

func (s *Service) ForgetFacts(keyword string) error {
	return s.memMgr.Store().DeleteFactsByKeyword(keyword)
}
