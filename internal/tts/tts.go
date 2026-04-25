package tts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bytectlgo/edge-tts/pkg/edge_tts"
)

type Service struct {
	voice   string
	enabled bool
}

func NewService(voice string, enabled bool) *Service {
	return &Service{
		voice:   voice,
		enabled: enabled,
	}
}

func (s *Service) Enabled() bool {
	return s.enabled
}

func (s *Service) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if !s.enabled {
		return nil, fmt.Errorf("tts is disabled")
	}

	tmpFile := filepath.Join(os.TempDir(), "bot_tts.mp3")
	defer os.Remove(tmpFile)

	comm := edge_tts.NewCommunicate(text, s.voice)
	if err := comm.Save(ctx, tmpFile, ""); err != nil {
		return nil, fmt.Errorf("tts synthesize: %w", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("read tts output: %w", err)
	}

	return data, nil
}
