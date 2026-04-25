package memory

import (
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}
	return s
}

func TestAddAndGetRecentMessages(t *testing.T) {
	s := testStore(t)

	msg1 := &Message{Role: "user", Content: "hello", ContentType: "text"}
	msg2 := &Message{Role: "assistant", Content: "world", ContentType: "text"}

	if err := s.AddMessage(msg1); err != nil {
		t.Fatalf("AddMessage() error: %v", err)
	}
	if err := s.AddMessage(msg2); err != nil {
		t.Fatalf("AddMessage() error: %v", err)
	}

	messages, err := s.GetRecentMessages(10)
	if err != nil {
		t.Fatalf("GetRecentMessages() error: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Content != "hello" {
		t.Errorf("expected first message content 'hello', got %q", messages[0].Content)
	}
	if messages[1].Content != "world" {
		t.Errorf("expected second message content 'world', got %q", messages[1].Content)
	}
}

func TestMessageCount(t *testing.T) {
	s := testStore(t)

	for i := 0; i < 3; i++ {
		msg := &Message{Role: "user", Content: "message", ContentType: "text"}
		if err := s.AddMessage(msg); err != nil {
			t.Fatalf("AddMessage() error: %v", err)
		}
	}

	count, err := s.MessageCount()
	if err != nil {
		t.Fatalf("MessageCount() error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestDeleteMessagesBefore(t *testing.T) {
	s := testStore(t)

	oldMsg := &Message{Role: "user", Content: "old message", ContentType: "text", CreatedAt: time.Now().Add(-2 * time.Second)}
	if err := s.AddMessage(oldMsg); err != nil {
		t.Fatalf("AddMessage() error: %v", err)
	}

	cutoff := time.Now().Add(-1 * time.Second)

	newMsg := &Message{Role: "user", Content: "new message", ContentType: "text"}
	if err := s.AddMessage(newMsg); err != nil {
		t.Fatalf("AddMessage() error: %v", err)
	}

	if err := s.DeleteMessagesBefore(cutoff); err != nil {
		t.Fatalf("DeleteMessagesBefore() error: %v", err)
	}

	messages, err := s.GetRecentMessages(10)
	if err != nil {
		t.Fatalf("GetRecentMessages() error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message after delete, got %d", len(messages))
	}
	if messages[0].Content != "new message" {
		t.Errorf("expected remaining message content 'new message', got %q", messages[0].Content)
	}
}

func TestAddAndGetSummaries(t *testing.T) {
	s := testStore(t)

	summary := &Summary{
		Content:  "test summary",
		FromTime: time.Now().Add(-24 * time.Hour),
		ToTime:   time.Now(),
	}
	if err := s.AddSummary(summary); err != nil {
		t.Fatalf("AddSummary() error: %v", err)
	}

	summaries, err := s.GetRecentSummaries(30)
	if err != nil {
		t.Fatalf("GetRecentSummaries() error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Content != "test summary" {
		t.Errorf("expected content 'test summary', got %q", summaries[0].Content)
	}
}

func TestAddAndGetFacts(t *testing.T) {
	s := testStore(t)

	fact1 := &Fact{Content: "the sky is blue", Category: "nature"}
	fact2 := &Fact{Content: "water is wet", Category: "nature"}

	if err := s.AddFact(fact1); err != nil {
		t.Fatalf("AddFact() error: %v", err)
	}
	if err := s.AddFact(fact2); err != nil {
		t.Fatalf("AddFact() error: %v", err)
	}

	facts, err := s.GetAllFacts()
	if err != nil {
		t.Fatalf("GetAllFacts() error: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
}

func TestDeleteFactsByKeyword(t *testing.T) {
	s := testStore(t)

	fact1 := &Fact{Content: "the sky is blue"}
	fact2 := &Fact{Content: "water is wet"}

	if err := s.AddFact(fact1); err != nil {
		t.Fatalf("AddFact() error: %v", err)
	}
	if err := s.AddFact(fact2); err != nil {
		t.Fatalf("AddFact() error: %v", err)
	}

	if err := s.DeleteFactsByKeyword("sky"); err != nil {
		t.Fatalf("DeleteFactsByKeyword() error: %v", err)
	}

	facts, err := s.GetAllFacts()
	if err != nil {
		t.Fatalf("GetAllFacts() error: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact after delete, got %d", len(facts))
	}
	if facts[0].Content != "water is wet" {
		t.Errorf("expected remaining fact 'water is wet', got %q", facts[0].Content)
	}
}

func TestClearAll(t *testing.T) {
	s := testStore(t)

	msg := &Message{Role: "user", Content: "hello", ContentType: "text"}
	fact := &Fact{Content: "some fact"}
	summary := &Summary{Content: "some summary"}

	if err := s.AddMessage(msg); err != nil {
		t.Fatalf("AddMessage() error: %v", err)
	}
	if err := s.AddFact(fact); err != nil {
		t.Fatalf("AddFact() error: %v", err)
	}
	if err := s.AddSummary(summary); err != nil {
		t.Fatalf("AddSummary() error: %v", err)
	}

	if err := s.ClearAll(); err != nil {
		t.Fatalf("ClearAll() error: %v", err)
	}

	count, err := s.MessageCount()
	if err != nil {
		t.Fatalf("MessageCount() error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 messages after ClearAll, got %d", count)
	}

	facts, err := s.GetAllFacts()
	if err != nil {
		t.Fatalf("GetAllFacts() error: %v", err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts after ClearAll, got %d", len(facts))
	}

	summaries, err := s.GetRecentSummaries(30)
	if err != nil {
		t.Fatalf("GetRecentSummaries() error: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries after ClearAll, got %d", len(summaries))
	}
}
