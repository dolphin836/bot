package memory

import (
	"context"
	"testing"
)

type mockCompressor struct {
	summarizeResult string
	factsResult     []string
}

func (m *mockCompressor) Summarize(ctx context.Context, messages []Message) (string, error) {
	return m.summarizeResult, nil
}

func (m *mockCompressor) ExtractFacts(ctx context.Context, messages []Message) ([]string, error) {
	return m.factsResult, nil
}

func TestBuildContextUnderLimit(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, ManagerConfig{RecentLimit: 50, SummaryMaxAgeDays: 30})

	msg1 := &Message{Role: "user", Content: "hello", ContentType: "text"}
	msg2 := &Message{Role: "assistant", Content: "world", ContentType: "text"}

	if err := s.AddMessage(msg1); err != nil {
		t.Fatalf("AddMessage() error: %v", err)
	}
	if err := s.AddMessage(msg2); err != nil {
		t.Fatalf("AddMessage() error: %v", err)
	}

	ctx := context.Background()
	cc, err := mgr.BuildContext(ctx)
	if err != nil {
		t.Fatalf("BuildContext() error: %v", err)
	}

	if len(cc.RecentMessages) != 2 {
		t.Errorf("expected 2 recent messages, got %d", len(cc.RecentMessages))
	}
}

func TestBuildContextIncludesFactsAndSummaries(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, ManagerConfig{RecentLimit: 50, SummaryMaxAgeDays: 30})

	fact := &Fact{Content: "the sky is blue", Category: "nature"}
	if err := s.AddFact(fact); err != nil {
		t.Fatalf("AddFact() error: %v", err)
	}

	summary := &Summary{Content: "previous conversation summary"}
	if err := s.AddSummary(summary); err != nil {
		t.Fatalf("AddSummary() error: %v", err)
	}

	msg := &Message{Role: "user", Content: "hello", ContentType: "text"}
	if err := s.AddMessage(msg); err != nil {
		t.Fatalf("AddMessage() error: %v", err)
	}

	ctx := context.Background()
	cc, err := mgr.BuildContext(ctx)
	if err != nil {
		t.Fatalf("BuildContext() error: %v", err)
	}

	if len(cc.Facts) != 1 {
		t.Errorf("expected 1 fact, got %d", len(cc.Facts))
	}
	if len(cc.Summaries) != 1 {
		t.Errorf("expected 1 summary, got %d", len(cc.Summaries))
	}
	if len(cc.RecentMessages) != 1 {
		t.Errorf("expected 1 recent message, got %d", len(cc.RecentMessages))
	}
}

func TestCompressTriggersWhenOverLimit(t *testing.T) {
	s := testStore(t)
	mock := &mockCompressor{
		summarizeResult: "summary of old messages",
		factsResult:     []string{"user likes go programming"},
	}
	mgr := NewManager(s, mock, ManagerConfig{RecentLimit: 3, SummaryMaxAgeDays: 30})

	for i := 0; i < 5; i++ {
		msg := &Message{Role: "user", Content: "message", ContentType: "text"}
		if err := s.AddMessage(msg); err != nil {
			t.Fatalf("AddMessage() error: %v", err)
		}
	}

	ctx := context.Background()
	if err := mgr.CompressIfNeeded(ctx); err != nil {
		t.Fatalf("CompressIfNeeded() error: %v", err)
	}

	count, err := s.MessageCount()
	if err != nil {
		t.Fatalf("MessageCount() error: %v", err)
	}
	if count > 3 {
		t.Errorf("expected message count <= 3 after compression, got %d", count)
	}

	summaries, err := s.GetRecentSummaries(30)
	if err != nil {
		t.Fatalf("GetRecentSummaries() error: %v", err)
	}
	if len(summaries) != 1 {
		t.Errorf("expected 1 summary after compression, got %d", len(summaries))
	}

	facts, err := s.GetAllFacts()
	if err != nil {
		t.Fatalf("GetAllFacts() error: %v", err)
	}
	if len(facts) != 1 {
		t.Errorf("expected 1 fact after compression, got %d", len(facts))
	}
}
