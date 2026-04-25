package tts

import (
	"context"
	"testing"
)

func TestSynthesize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	svc := NewService("zh-CN-XiaoyiNeural", true)
	data, err := svc.Synthesize(context.Background(), "你好，今天天气不错")
	if err != nil {
		t.Fatalf("Synthesize() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty audio data")
	}
	t.Logf("audio size: %d bytes", len(data))
}
