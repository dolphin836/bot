package memory

import (
	"context"
	"time"
)

// Compressor is implemented by the LLM layer to generate summaries and extract facts.
type Compressor interface {
	Summarize(ctx context.Context, messages []Message) (string, error)
	ExtractFacts(ctx context.Context, messages []Message) ([]string, error)
}

type ManagerConfig struct {
	RecentLimit       int
	SummaryMaxAgeDays int
}

type Manager struct {
	store      *Store
	compressor Compressor
	config     ManagerConfig
}

type ImageContent struct {
	Base64    string
	MediaType string
	Caption   string
}

type ConversationContext struct {
	Facts          []Fact
	Summaries      []Summary
	RecentMessages []Message
	ImageData      *ImageContent
}

func NewManager(store *Store, compressor Compressor, config ManagerConfig) *Manager {
	return &Manager{
		store:      store,
		compressor: compressor,
		config:     config,
	}
}

// BuildContext fetches all facts, recent summaries (by max age days), and recent messages (by limit).
func (m *Manager) BuildContext(ctx context.Context) (*ConversationContext, error) {
	facts, err := m.store.GetAllFacts()
	if err != nil {
		return nil, err
	}

	summaries, err := m.store.GetRecentSummaries(m.config.SummaryMaxAgeDays)
	if err != nil {
		return nil, err
	}

	messages, err := m.store.GetRecentMessages(m.config.RecentLimit)
	if err != nil {
		return nil, err
	}

	return &ConversationContext{
		Facts:          facts,
		Summaries:      summaries,
		RecentMessages: messages,
	}, nil
}

// CompressIfNeeded compresses old messages if message count exceeds RecentLimit.
// If compressor is nil, it skips compression.
func (m *Manager) CompressIfNeeded(ctx context.Context) error {
	if m.compressor == nil {
		return nil
	}

	count, err := m.store.MessageCount()
	if err != nil {
		return err
	}

	if count <= int64(m.config.RecentLimit) {
		return nil
	}

	// Determine how many messages to compress (overflow = count - limit)
	overflowCount := int(count) - m.config.RecentLimit
	overflow, err := m.store.GetOldestMessages(overflowCount)
	if err != nil {
		return err
	}

	if len(overflow) == 0 {
		return nil
	}

	// Generate summary from overflow messages
	summaryContent, err := m.compressor.Summarize(ctx, overflow)
	if err != nil {
		return err
	}

	// Determine time range for summary
	fromTime := overflow[0].CreatedAt
	toTime := overflow[len(overflow)-1].CreatedAt

	summary := &Summary{
		Content:  summaryContent,
		FromTime: fromTime,
		ToTime:   toTime,
	}
	if err := m.store.AddSummary(summary); err != nil {
		return err
	}

	// Extract facts from overflow messages
	facts, err := m.compressor.ExtractFacts(ctx, overflow)
	if err != nil {
		return err
	}

	for _, factContent := range facts {
		fact := &Fact{
			Content:   factContent,
			CreatedAt: time.Now(),
		}
		if err := m.store.AddFact(fact); err != nil {
			return err
		}
	}

	// Delete compressed messages
	ids := make([]uint, len(overflow))
	for i, msg := range overflow {
		ids[i] = msg.ID
	}
	if err := m.store.DeleteMessagesByIDs(ids); err != nil {
		return err
	}

	return nil
}

// Store returns the underlying store.
func (m *Manager) Store() *Store {
	return m.store
}
