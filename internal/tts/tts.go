package tts

import (
	"context"
	"fmt"

	"github.com/lib-x/edgetts"
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

	client := edgetts.New(edgetts.WithVoice(s.voice))
	data, err := client.Bytes(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("tts synthesize: %w", err)
	}
	return data, nil
}
