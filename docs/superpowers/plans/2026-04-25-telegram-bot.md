# Telegram Personal Assistant Bot — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go-based personal Telegram assistant powered by Claude API with streaming responses, three-tier memory, and extensible tool use.

**Architecture:** Layered architecture — bot (Telegram I/O) → chat (orchestration) → memory (SQLite persistence + context assembly) → llm (Claude API streaming + tool dispatch). Single user, single perpetual conversation.

**Tech Stack:** Go, `github.com/go-telegram/bot`, `github.com/anthropics/anthropic-sdk-go`, `github.com/glebarez/sqlite` (pure-Go GORM driver), `github.com/spf13/viper`, `log/slog`

---

## File Map

| File | Responsibility |
|---|---|
| `cmd/bot/main.go` | Entry point, config load, component wiring, graceful shutdown |
| `config.yaml` | Default config template (secrets via env) |
| `internal/config/config.go` | Config struct, viper loading |
| `internal/config/config_test.go` | Config loading tests |
| `internal/memory/models.go` | GORM model structs (Message, Summary, Fact, ToolCall) |
| `internal/memory/store.go` | SQLite CRUD operations |
| `internal/memory/store_test.go` | Store tests |
| `internal/memory/manager.go` | Three-tier context assembly, compression trigger |
| `internal/memory/manager_test.go` | Manager tests |
| `internal/llm/client.go` | Claude API wrapper, streaming, message building |
| `internal/llm/client_test.go` | Client tests |
| `internal/llm/tools.go` | Tool use loop: detect tool_use, dispatch, feed back result |
| `internal/tools/registry.go` | Tool interface, registry (register/lookup/list) |
| `internal/tools/registry_test.go` | Registry tests |
| `internal/bot/handler.go` | Telegram update handler, auth gate, message routing |
| `internal/bot/commands.go` | /clear /help /facts /forget handlers |
| `internal/chat/service.go` | Orchestration: receive msg → memory → llm → stream → save |

---

### Task 1: Project Scaffold & Dependencies

**Files:**
- Create: `go.mod`, `go.sum`, `config.yaml`, `.gitignore`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/eric/Work/Code/Bot
go mod init github.com/dolphin836/bot
```

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/go-telegram/bot@latest
go get github.com/anthropics/anthropic-sdk-go@latest
go get gorm.io/gorm@latest
go get github.com/glebarez/sqlite@latest
go get github.com/spf13/viper@latest
go get github.com/invopop/jsonschema@latest
```

- [ ] **Step 3: Create config.yaml**

```yaml
telegram:
  token: ${TELEGRAM_BOT_TOKEN}
  owner_id: 0

anthropic:
  api_key: ${ANTHROPIC_API_KEY}
  model: claude-sonnet-4-20250514

memory:
  recent_limit: 50
  summary_max_age_days: 30

db:
  path: ./data/bot.db
```

- [ ] **Step 4: Create .gitignore**

```
data/
*.db
config.local.yaml
.env
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum config.yaml .gitignore
git commit -m "feat: initialize Go module with dependencies and config template"
```

---

### Task 2: Config Loading

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
telegram:
  token: "test-token"
  owner_id: 12345
anthropic:
  api_key: "test-key"
  model: "claude-sonnet-4-20250514"
memory:
  recent_limit: 30
  summary_max_age_days: 7
db:
  path: "./data/test.db"
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Telegram.Token != "test-token" {
		t.Errorf("token = %q, want %q", cfg.Telegram.Token, "test-token")
	}
	if cfg.Telegram.OwnerID != 12345 {
		t.Errorf("owner_id = %d, want %d", cfg.Telegram.OwnerID, 12345)
	}
	if cfg.Anthropic.Model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q, want %q", cfg.Anthropic.Model, "claude-sonnet-4-20250514")
	}
	if cfg.Memory.RecentLimit != 30 {
		t.Errorf("recent_limit = %d, want %d", cfg.Memory.RecentLimit, 30)
	}
}

func TestLoadFromEnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
telegram:
  token: "file-token"
  owner_id: 0
anthropic:
  api_key: "file-key"
  model: "claude-sonnet-4-20250514"
memory:
  recent_limit: 50
  summary_max_age_days: 30
db:
  path: "./data/bot.db"
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("TELEGRAM_BOT_TOKEN", "env-token")
	t.Setenv("ANTHROPIC_API_KEY", "env-key")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Telegram.Token != "env-token" {
		t.Errorf("token = %q, want %q", cfg.Telegram.Token, "env-token")
	}
	if cfg.Anthropic.APIKey != "env-key" {
		t.Errorf("api_key = %q, want %q", cfg.Anthropic.APIKey, "env-key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write implementation**

```go
// internal/config/config.go
package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Telegram  TelegramConfig  `mapstructure:"telegram"`
	Anthropic AnthropicConfig `mapstructure:"anthropic"`
	Memory    MemoryConfig    `mapstructure:"memory"`
	DB        DBConfig        `mapstructure:"db"`
}

type TelegramConfig struct {
	Token   string `mapstructure:"token"`
	OwnerID int64  `mapstructure:"owner_id"`
}

type AnthropicConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type MemoryConfig struct {
	RecentLimit      int `mapstructure:"recent_limit"`
	SummaryMaxAgeDays int `mapstructure:"summary_max_age_days"`
}

type DBConfig struct {
	Path string `mapstructure:"path"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.BindEnv("telegram.token", "TELEGRAM_BOT_TOKEN")
	v.BindEnv("anthropic.api_key", "ANTHROPIC_API_KEY")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config loading with viper (env override support)"
```

---

### Task 3: Memory Store (SQLite Models + CRUD)

**Files:**
- Create: `internal/memory/models.go`
- Create: `internal/memory/store.go`
- Create: `internal/memory/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/memory/store_test.go
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

	s.AddMessage(&Message{Role: "user", Content: "hello", ContentType: "text"})
	s.AddMessage(&Message{Role: "assistant", Content: "hi there", ContentType: "text"})

	msgs, err := s.GetRecentMessages(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("msgs[0].Content = %q, want %q", msgs[0].Content, "hello")
	}
	if msgs[1].Content != "hi there" {
		t.Errorf("msgs[1].Content = %q, want %q", msgs[1].Content, "hi there")
	}
}

func TestMessageCount(t *testing.T) {
	s := testStore(t)

	s.AddMessage(&Message{Role: "user", Content: "one", ContentType: "text"})
	s.AddMessage(&Message{Role: "assistant", Content: "two", ContentType: "text"})
	s.AddMessage(&Message{Role: "user", Content: "three", ContentType: "text"})

	count, err := s.MessageCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestDeleteMessagesBefore(t *testing.T) {
	s := testStore(t)

	s.AddMessage(&Message{Role: "user", Content: "old", ContentType: "text"})
	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)
	s.AddMessage(&Message{Role: "user", Content: "new", ContentType: "text"})

	err := s.DeleteMessagesBefore(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := s.GetRecentMessages(10)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if msgs[0].Content != "new" {
		t.Errorf("content = %q, want %q", msgs[0].Content, "new")
	}
}

func TestAddAndGetSummaries(t *testing.T) {
	s := testStore(t)

	now := time.Now()
	s.AddSummary(&Summary{
		Content:  "discussed Go project",
		FromTime: now.Add(-1 * time.Hour),
		ToTime:   now,
	})

	summaries, err := s.GetRecentSummaries(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want 1", len(summaries))
	}
	if summaries[0].Content != "discussed Go project" {
		t.Errorf("content = %q, want expected", summaries[0].Content)
	}
}

func TestAddAndGetFacts(t *testing.T) {
	s := testStore(t)

	s.AddFact(&Fact{Content: "user likes Go", Category: "preference"})
	s.AddFact(&Fact{Content: "user has a cat", Category: "personal"})

	facts, err := s.GetAllFacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("got %d facts, want 2", len(facts))
	}
}

func TestDeleteFactsByKeyword(t *testing.T) {
	s := testStore(t)

	s.AddFact(&Fact{Content: "user likes Go", Category: "preference"})
	s.AddFact(&Fact{Content: "user has a cat named Mimi", Category: "personal"})

	err := s.DeleteFactsByKeyword("cat")
	if err != nil {
		t.Fatal(err)
	}
	facts, _ := s.GetAllFacts()
	if len(facts) != 1 {
		t.Fatalf("got %d facts, want 1", len(facts))
	}
	if facts[0].Content != "user likes Go" {
		t.Errorf("remaining fact = %q", facts[0].Content)
	}
}

func TestClearAll(t *testing.T) {
	s := testStore(t)

	s.AddMessage(&Message{Role: "user", Content: "hi", ContentType: "text"})
	s.AddFact(&Fact{Content: "something", Category: "test"})
	s.AddSummary(&Summary{Content: "summary", FromTime: time.Now(), ToTime: time.Now()})

	err := s.ClearAll()
	if err != nil {
		t.Fatal(err)
	}

	msgs, _ := s.GetRecentMessages(10)
	facts, _ := s.GetAllFacts()
	summaries, _ := s.GetRecentSummaries(30)
	if len(msgs) != 0 || len(facts) != 0 || len(summaries) != 0 {
		t.Error("expected all tables to be empty after ClearAll")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/memory/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write models**

```go
// internal/memory/models.go
package memory

import (
	"time"
)

type Message struct {
	ID          uint   `gorm:"primarykey"`
	Role        string `gorm:"not null"`
	Content     string `gorm:"not null"`
	ContentType string `gorm:"not null;default:text"`
	TokensIn    int
	TokensOut   int
	CreatedAt   time.Time
}

type Summary struct {
	ID        uint   `gorm:"primarykey"`
	Content   string `gorm:"not null"`
	FromTime  time.Time
	ToTime    time.Time
	CreatedAt time.Time
}

type Fact struct {
	ID        uint   `gorm:"primarykey"`
	Content   string `gorm:"not null"`
	Category  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ToolCallRecord struct {
	ID        uint   `gorm:"primarykey"`
	MessageID uint   `gorm:"index"`
	ToolName  string `gorm:"not null"`
	Input     string
	Output    string
	Status    string `gorm:"not null;default:pending"`
	CreatedAt time.Time
}
```

- [ ] **Step 4: Write store implementation**

```go
// internal/memory/store.go
package memory

import (
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db *gorm.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&Message{}, &Summary{}, &Fact{}, &ToolCallRecord{}); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) AddMessage(msg *Message) error {
	return s.db.Create(msg).Error
}

func (s *Store) GetRecentMessages(limit int) ([]Message, error) {
	var msgs []Message
	err := s.db.Order("id asc").Limit(limit).Find(&msgs).Error
	return msgs, err
}

func (s *Store) GetOldestMessages(limit int) ([]Message, error) {
	var msgs []Message
	err := s.db.Order("id asc").Limit(limit).Find(&msgs).Error
	return msgs, err
}

func (s *Store) MessageCount() (int64, error) {
	var count int64
	err := s.db.Model(&Message{}).Count(&count).Error
	return count, err
}

func (s *Store) DeleteMessagesBefore(t time.Time) error {
	return s.db.Where("created_at < ?", t).Delete(&Message{}).Error
}

func (s *Store) DeleteMessagesByIDs(ids []uint) error {
	return s.db.Delete(&Message{}, ids).Error
}

func (s *Store) AddSummary(summary *Summary) error {
	return s.db.Create(summary).Error
}

func (s *Store) GetRecentSummaries(maxAgeDays int) ([]Summary, error) {
	var summaries []Summary
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	err := s.db.Where("created_at > ?", cutoff).Order("id asc").Find(&summaries).Error
	return summaries, err
}

func (s *Store) AddFact(fact *Fact) error {
	return s.db.Create(fact).Error
}

func (s *Store) GetAllFacts() ([]Fact, error) {
	var facts []Fact
	err := s.db.Order("id asc").Find(&facts).Error
	return facts, err
}

func (s *Store) DeleteFactsByKeyword(keyword string) error {
	return s.db.Where("content LIKE ?", "%"+keyword+"%").Delete(&Fact{}).Error
}

func (s *Store) AddToolCall(tc *ToolCallRecord) error {
	return s.db.Create(tc).Error
}

func (s *Store) UpdateToolCall(id uint, output string, status string) error {
	return s.db.Model(&ToolCallRecord{}).Where("id = ?", id).Updates(map[string]any{
		"output": output,
		"status": status,
	}).Error
}

func (s *Store) ClearAll() error {
	if err := s.db.Where("1 = 1").Delete(&Message{}).Error; err != nil {
		return err
	}
	if err := s.db.Where("1 = 1").Delete(&Summary{}).Error; err != nil {
		return err
	}
	if err := s.db.Where("1 = 1").Delete(&Fact{}).Error; err != nil {
		return err
	}
	return s.db.Where("1 = 1").Delete(&ToolCallRecord{}).Error
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/memory/ -v -run "Test(Add|Message|Delete|Clear)"`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/memory/
git commit -m "feat: add SQLite memory store with GORM models and CRUD"
```

---

### Task 4: Memory Manager (Three-Tier Context Assembly)

**Files:**
- Create: `internal/memory/manager.go`
- Create: `internal/memory/manager_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/memory/manager_test.go
package memory

import (
	"context"
	"testing"
)

// mockCompressor implements the Compressor interface for testing
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

	s.AddMessage(&Message{Role: "user", Content: "hello", ContentType: "text"})
	s.AddMessage(&Message{Role: "assistant", Content: "hi", ContentType: "text"})

	ctx := context.Background()
	result, err := mgr.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RecentMessages) != 2 {
		t.Errorf("got %d recent messages, want 2", len(result.RecentMessages))
	}
}

func TestBuildContextIncludesFactsAndSummaries(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, ManagerConfig{RecentLimit: 50, SummaryMaxAgeDays: 30})

	s.AddFact(&Fact{Content: "user likes Go", Category: "preference"})
	s.AddSummary(&Summary{Content: "discussed project architecture"})
	s.AddMessage(&Message{Role: "user", Content: "hello", ContentType: "text"})

	ctx := context.Background()
	result, err := mgr.BuildContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Facts) != 1 {
		t.Errorf("got %d facts, want 1", len(result.Facts))
	}
	if len(result.Summaries) != 1 {
		t.Errorf("got %d summaries, want 1", len(result.Summaries))
	}
}

func TestCompressTriggersWhenOverLimit(t *testing.T) {
	s := testStore(t)
	comp := &mockCompressor{
		summarizeResult: "summary of old messages",
		factsResult:     []string{"user likes testing"},
	}
	mgr := NewManager(s, comp, ManagerConfig{RecentLimit: 3, SummaryMaxAgeDays: 30})

	// Add 5 messages (over limit of 3)
	for i := 0; i < 5; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		s.AddMessage(&Message{Role: role, Content: "msg", ContentType: "text"})
	}

	ctx := context.Background()
	err := mgr.CompressIfNeeded(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// After compression, messages should be at or below limit
	count, _ := s.MessageCount()
	if count > 3 {
		t.Errorf("message count after compress = %d, want <= 3", count)
	}

	// Summary should have been created
	summaries, _ := s.GetRecentSummaries(30)
	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want 1", len(summaries))
	}
	if summaries[0].Content != "summary of old messages" {
		t.Errorf("summary = %q", summaries[0].Content)
	}

	// Fact should have been created
	facts, _ := s.GetAllFacts()
	if len(facts) != 1 {
		t.Fatalf("got %d facts, want 1", len(facts))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/memory/ -v -run "TestBuild|TestCompress"`
Expected: FAIL — types not defined

- [ ] **Step 3: Write implementation**

```go
// internal/memory/manager.go
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

type ConversationContext struct {
	Facts          []Fact
	Summaries      []Summary
	RecentMessages []Message
}

func NewManager(store *Store, compressor Compressor, config ManagerConfig) *Manager {
	return &Manager{
		store:      store,
		compressor: compressor,
		config:     config,
	}
}

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

func (m *Manager) CompressIfNeeded(ctx context.Context) error {
	count, err := m.store.MessageCount()
	if err != nil {
		return err
	}

	if int(count) <= m.config.RecentLimit {
		return nil
	}

	if m.compressor == nil {
		return nil
	}

	overflow := int(count) - m.config.RecentLimit
	oldMsgs, err := m.store.GetOldestMessages(overflow)
	if err != nil {
		return err
	}
	if len(oldMsgs) == 0 {
		return nil
	}

	summary, err := m.compressor.Summarize(ctx, oldMsgs)
	if err != nil {
		return err
	}

	err = m.store.AddSummary(&Summary{
		Content:  summary,
		FromTime: oldMsgs[0].CreatedAt,
		ToTime:   oldMsgs[len(oldMsgs)-1].CreatedAt,
	})
	if err != nil {
		return err
	}

	factStrings, err := m.compressor.ExtractFacts(ctx, oldMsgs)
	if err != nil {
		return err
	}
	for _, f := range factStrings {
		err = m.store.AddFact(&Fact{
			Content:   f,
			Category:  "auto",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		if err != nil {
			return err
		}
	}

	ids := make([]uint, len(oldMsgs))
	for i, msg := range oldMsgs {
		ids[i] = msg.ID
	}
	return m.store.DeleteMessagesByIDs(ids)
}

func (m *Manager) Store() *Store {
	return m.store
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/memory/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/memory/
git commit -m "feat: add memory manager with three-tier context assembly and compression"
```

---

### Task 5: Tool Registry

**Files:**
- Create: `internal/tools/registry.go`
- Create: `internal/tools/registry_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/tools/registry_test.go
package tools

import (
	"context"
	"encoding/json"
	"testing"
)

type echoTool struct{}

func (e *echoTool) Name() string        { return "echo" }
func (e *echoTool) Description() string { return "Echoes the input" }
func (e *echoTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)
}
func (e *echoTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var req struct {
		Text string `json:"text"`
	}
	json.Unmarshal(input, &req)
	return req.Text, nil
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})

	tool, ok := r.Get("echo")
	if !ok {
		t.Fatal("expected to find echo tool")
	}
	if tool.Name() != "echo" {
		t.Errorf("name = %q, want %q", tool.Name(), "echo")
	}
}

func TestGetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestExecute(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})

	result, err := r.Execute(context.Background(), "echo", json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello" {
		t.Errorf("result = %q, want %q", result, "hello")
	}
}

func TestListTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})

	tools := r.List()
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

```go
// internal/tools/registry.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}
	return tool.Execute(ctx, input)
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tools/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/
git commit -m "feat: add tool registry with interface, register, lookup, and execute"
```

---

### Task 6: Claude LLM Client (Streaming + Tool Use)

**Files:**
- Create: `internal/llm/client.go`
- Create: `internal/llm/tools.go`
- Create: `internal/llm/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/llm/client_test.go
package llm

import (
	"testing"

	"github.com/dolphin836/bot/internal/memory"
	"github.com/dolphin836/bot/internal/tools"
)

func TestBuildMessagesFromContext(t *testing.T) {
	ctx := &memory.ConversationContext{
		Facts: []memory.Fact{
			{Content: "user likes Go"},
		},
		Summaries: []memory.Summary{
			{Content: "discussed architecture"},
		},
		RecentMessages: []memory.Message{
			{Role: "user", Content: "hello", ContentType: "text"},
			{Role: "assistant", Content: "hi there", ContentType: "text"},
		},
	}

	systemPrompt, messages := BuildMessages(ctx)

	if systemPrompt == "" {
		t.Error("system prompt should not be empty")
	}

	if len(messages) != 2 {
		t.Errorf("got %d messages, want 2", len(messages))
	}
}

func TestBuildToolParams(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&tools.EchoTestTool{})

	params := BuildToolParams(r)
	if len(params) != 1 {
		t.Fatalf("got %d tool params, want 1", len(params))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/ -v`
Expected: FAIL

- [ ] **Step 3: Add EchoTestTool helper to tools package for testing**

```go
// Add to internal/tools/registry.go (at the bottom, exported for testing by other packages)

// EchoTestTool is exported for use in tests across packages.
type EchoTestTool struct{}

func (e *EchoTestTool) Name() string        { return "echo" }
func (e *EchoTestTool) Description() string { return "Echoes the input" }
func (e *EchoTestTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`)
}
func (e *EchoTestTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var req struct {
		Text string `json:"text"`
	}
	json.Unmarshal(input, &req)
	return req.Text, nil
}
```

- [ ] **Step 4: Write LLM client implementation**

```go
// internal/llm/client.go
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/dolphin836/bot/internal/memory"
	"github.com/dolphin836/bot/internal/tools"
)

type StreamCallback func(text string)

type Client struct {
	client   anthropic.Client
	model    string
	registry *tools.Registry
}

func NewClient(apiKey string, model string, registry *tools.Registry) *Client {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Client{
		client:   client,
		model:    model,
		registry: registry,
	}
}

func BuildMessages(convCtx *memory.ConversationContext) (string, []anthropic.MessageParam) {
	var sb strings.Builder
	sb.WriteString("You are a helpful personal assistant. Be concise and direct.\n\n")

	if len(convCtx.Facts) > 0 {
		sb.WriteString("## Things I know about the user\n")
		for _, f := range convCtx.Facts {
			sb.WriteString("- ")
			sb.WriteString(f.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(convCtx.Summaries) > 0 {
		sb.WriteString("## Previous conversation summaries\n")
		for _, s := range convCtx.Summaries {
			sb.WriteString("- ")
			sb.WriteString(s.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	messages := make([]anthropic.MessageParam, 0, len(convCtx.RecentMessages))
	for _, msg := range convCtx.RecentMessages {
		switch msg.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		}
	}

	return sb.String(), messages
}

func BuildToolParams(registry *tools.Registry) []anthropic.ToolUnionParam {
	toolList := registry.List()
	if len(toolList) == 0 {
		return nil
	}

	params := make([]anthropic.ToolUnionParam, 0, len(toolList))
	for _, t := range toolList {
		var schema anthropic.ToolInputSchemaParam
		json.Unmarshal(t.InputSchema(), &schema)

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

func (c *Client) SendStreaming(
	ctx context.Context,
	convCtx *memory.ConversationContext,
	onText StreamCallback,
) (string, error) {
	systemPrompt, messages := BuildMessages(convCtx)
	toolParams := BuildToolParams(c.registry)

	return c.streamWithToolLoop(ctx, systemPrompt, messages, toolParams, onText)
}

func (c *Client) streamWithToolLoop(
	ctx context.Context,
	systemPrompt string,
	messages []anthropic.MessageParam,
	toolParams []anthropic.ToolUnionParam,
	onText StreamCallback,
) (string, error) {
	for {
		params := anthropic.MessageNewParams{
			Model:     c.model,
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
		}
		if len(toolParams) > 0 {
			params.Tools = toolParams
		}

		stream := c.client.Messages.NewStreaming(ctx, params)
		accumulated := anthropic.Message{}
		var fullText strings.Builder

		for stream.Next() {
			event := stream.Current()
			if err := accumulated.Accumulate(event); err != nil {
				return "", fmt.Errorf("accumulate: %w", err)
			}

			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch delta := ev.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					fullText.WriteString(delta.Text)
					if onText != nil {
						onText(fullText.String())
					}
				}
			}
		}
		if stream.Err() != nil {
			return "", fmt.Errorf("stream: %w", stream.Err())
		}

		// Check if tool use was requested
		hasToolUse := false
		for _, block := range accumulated.Content {
			if _, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				hasToolUse = true
				break
			}
		}

		if !hasToolUse {
			return fullText.String(), nil
		}

		// Handle tool use
		messages = append(messages, accumulated.ToParam())
		toolResults := []anthropic.ContentBlockParamUnion{}

		for _, block := range accumulated.Content {
			if toolUse, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				slog.Info("tool_call", "tool", toolUse.Name, "id", toolUse.ID)

				inputJSON, _ := json.Marshal(toolUse.Input)
				result, err := c.registry.Execute(ctx, toolUse.Name, inputJSON)
				isError := false
				if err != nil {
					result = err.Error()
					isError = true
					slog.Error("tool_error", "tool", toolUse.Name, "error", err)
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(toolUse.ID, result, isError))
			}
		}

		messages = append(messages, anthropic.NewUserMessage(toolResults...))
		fullText.Reset()
	}
}
```

- [ ] **Step 5: Write Compressor implementation (for memory manager)**

```go
// internal/llm/tools.go
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/dolphin836/bot/internal/memory"
)

// Compressor uses Claude to generate summaries and extract facts.
type Compressor struct {
	client anthropic.Client
	model  string
}

func NewCompressor(apiKey string, model string) *Compressor {
	return &Compressor{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (c *Compressor) Summarize(ctx context.Context, messages []memory.Message) (string, error) {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:    c.model,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: "Summarize the following conversation segment concisely. Capture key topics, decisions, and outcomes. Write in 2-4 sentences."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(sb.String())),
		},
	})
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			return text.Text, nil
		}
	}
	return "", fmt.Errorf("summarize: no text in response")
}

func (c *Compressor) ExtractFacts(ctx context.Context, messages []memory.Message) ([]string, error) {
	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}

	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:    c.model,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: "Extract key facts, preferences, and important information about the user from this conversation. Return each fact on its own line, prefixed with '- '. Only include facts worth remembering long-term. If there are no notable facts, return an empty response."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(sb.String())),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("extract facts: %w", err)
	}

	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			if strings.TrimSpace(text.Text) == "" {
				return nil, nil
			}
			lines := strings.Split(text.Text, "\n")
			var facts []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				line = strings.TrimPrefix(line, "- ")
				line = strings.TrimSpace(line)
				if line != "" {
					facts = append(facts, line)
				}
			}
			return facts, nil
		}
	}
	return nil, nil
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/llm/ -v`
Expected: PASS (tests only cover BuildMessages and BuildToolParams, no API calls)

- [ ] **Step 7: Commit**

```bash
git add internal/llm/ internal/tools/
git commit -m "feat: add Claude LLM client with streaming, tool use loop, and compressor"
```

---

### Task 7: Telegram Bot Handler

**Files:**
- Create: `internal/bot/handler.go`

- [ ] **Step 1: Write implementation**

```go
// internal/bot/handler.go
package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dolphin836/bot/internal/chat"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type Handler struct {
	ownerID int64
	chatSvc *chat.Service
	mu      sync.Mutex
}

func NewHandler(ownerID int64, chatSvc *chat.Service) *Handler {
	return &Handler{
		ownerID: ownerID,
		chatSvc: chatSvc,
	}
}

func (h *Handler) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	msg := update.Message

	if msg.From == nil || msg.From.ID != h.ownerID {
		return
	}

	// Handle commands
	if msg.Text != "" && strings.HasPrefix(msg.Text, "/") {
		h.handleCommand(ctx, b, msg)
		return
	}

	// Handle text messages
	if msg.Text != "" {
		h.handleText(ctx, b, msg)
		return
	}

	// Handle photos
	if len(msg.Photo) > 0 {
		h.handlePhoto(ctx, b, msg)
		return
	}
}

func (h *Handler) handleText(ctx context.Context, b *bot.Bot, msg *models.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "⏳",
	})
	if err != nil {
		slog.Error("send_placeholder", "error", err)
		return
	}

	var lastEdit time.Time
	callback := func(text string) {
		if time.Since(lastEdit) < time.Second {
			return
		}
		lastEdit = time.Now()
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: sent.ID,
			Text:      text,
		})
	}

	reply, err := h.chatSvc.HandleMessage(ctx, msg.Text, "text", callback)
	if err != nil {
		slog.Error("handle_message", "error", err)
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: sent.ID,
			Text:      fmt.Sprintf("Error: %v", err),
		})
		return
	}

	b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: sent.ID,
		Text:      reply,
	})
}

func (h *Handler) handlePhoto(ctx context.Context, b *bot.Bot, msg *models.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Get the largest photo
	photo := msg.Photo[len(msg.Photo)-1]

	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: photo.FileID})
	if err != nil {
		slog.Error("get_file", "error", err)
		return
	}

	downloadURL := b.FileDownloadLink(file)
	resp, err := http.Get(downloadURL)
	if err != nil {
		slog.Error("download_file", "error", err)
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("read_file", "error", err)
		return
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	caption := msg.Caption
	if caption == "" {
		caption = "What's in this image?"
	}

	// Build content as: image_base64|||caption
	content := b64 + "|||" + caption

	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "⏳",
	})
	if err != nil {
		slog.Error("send_placeholder", "error", err)
		return
	}

	var lastEdit time.Time
	callback := func(text string) {
		if time.Since(lastEdit) < time.Second {
			return
		}
		lastEdit = time.Now()
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: sent.ID,
			Text:      text,
		})
	}

	reply, err := h.chatSvc.HandleMessage(ctx, content, "image", callback)
	if err != nil {
		slog.Error("handle_photo", "error", err)
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: sent.ID,
			Text:      fmt.Sprintf("Error: %v", err),
		})
		return
	}

	b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: sent.ID,
		Text:      reply,
	})
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/bot/handler.go
git commit -m "feat: add Telegram bot handler with auth, text, and photo support"
```

---

### Task 8: Bot Commands

**Files:**
- Create: `internal/bot/commands.go`

- [ ] **Step 1: Write implementation**

```go
// internal/bot/commands.go
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *Handler) handleCommand(ctx context.Context, b *bot.Bot, msg *models.Message) {
	parts := strings.Fields(msg.Text)
	cmd := parts[0]

	switch cmd {
	case "/help":
		h.cmdHelp(ctx, b, msg)
	case "/clear":
		h.cmdClear(ctx, b, msg)
	case "/facts":
		h.cmdFacts(ctx, b, msg)
	case "/forget":
		keyword := ""
		if len(parts) > 1 {
			keyword = strings.Join(parts[1:], " ")
		}
		h.cmdForget(ctx, b, msg, keyword)
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   fmt.Sprintf("Unknown command: %s\nType /help for available commands.", cmd),
		})
	}
}

func (h *Handler) cmdHelp(ctx context.Context, b *bot.Bot, msg *models.Message) {
	text := `/help — Show this message
/clear — Clear all memory and start fresh
/facts — Show stored long-term facts
/forget <keyword> — Delete facts matching keyword`

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   text,
	})
}

func (h *Handler) cmdClear(ctx context.Context, b *bot.Bot, msg *models.Message) {
	err := h.chatSvc.ClearAll()
	if err != nil {
		slog.Error("clear_all", "error", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   fmt.Sprintf("Error: %v", err),
		})
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "Memory cleared. Starting fresh.",
	})
}

func (h *Handler) cmdFacts(ctx context.Context, b *bot.Bot, msg *models.Message) {
	facts, err := h.chatSvc.GetFacts()
	if err != nil {
		slog.Error("get_facts", "error", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   fmt.Sprintf("Error: %v", err),
		})
		return
	}

	if len(facts) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "No facts stored yet.",
		})
		return
	}

	var sb strings.Builder
	sb.WriteString("Stored facts:\n\n")
	for i, f := range facts {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f.Content))
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   sb.String(),
	})
}

func (h *Handler) cmdForget(ctx context.Context, b *bot.Bot, msg *models.Message, keyword string) {
	if keyword == "" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Usage: /forget <keyword>",
		})
		return
	}

	err := h.chatSvc.ForgetFacts(keyword)
	if err != nil {
		slog.Error("forget_facts", "error", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   fmt.Sprintf("Error: %v", err),
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   fmt.Sprintf("Forgot facts matching %q.", keyword),
	})
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/bot/commands.go
git commit -m "feat: add bot commands (/help, /clear, /facts, /forget)"
```

---

### Task 9: Chat Service (Orchestration)

**Files:**
- Create: `internal/chat/service.go`

- [ ] **Step 1: Write implementation**

```go
// internal/chat/service.go
package chat

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
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
	// Save user message (for images, save the caption only)
	storeContent := content
	if contentType == "image" {
		parts := strings.SplitN(content, "|||", 2)
		if len(parts) == 2 {
			storeContent = "[image] " + parts[1]
		}
	}

	err := s.memMgr.Store().AddMessage(&memory.Message{
		Role:        "user",
		Content:     storeContent,
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}

	// Build context
	convCtx, err := s.memMgr.BuildContext(ctx)
	if err != nil {
		return "", err
	}

	// For image messages, replace last message with image content block
	if contentType == "image" {
		s.injectImageMessage(convCtx, content)
	}

	// Call Claude
	reply, err := s.llmClient.SendStreaming(ctx, convCtx, onStream)
	if err != nil {
		return "", err
	}

	// Save assistant response
	err = s.memMgr.Store().AddMessage(&memory.Message{
		Role:        "assistant",
		Content:     reply,
		ContentType: "text",
	})
	if err != nil {
		slog.Error("save_assistant_message", "error", err)
	}

	// Compress if needed (async-safe since we're already locked in handler)
	if err := s.memMgr.CompressIfNeeded(ctx); err != nil {
		slog.Error("compress", "error", err)
	}

	return reply, nil
}

func (s *Service) injectImageMessage(convCtx *memory.ConversationContext, content string) {
	parts := strings.SplitN(content, "|||", 2)
	if len(parts) != 2 {
		return
	}

	b64Data := parts[0]
	caption := parts[1]

	// Validate base64
	_, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return
	}

	// Replace the last message in context with an image-aware version
	if len(convCtx.RecentMessages) > 0 {
		last := &convCtx.RecentMessages[len(convCtx.RecentMessages)-1]
		last.Content = caption
		last.ContentType = "image"
		// Store base64 in a separate field for LLM to pick up
		// We'll handle this in the LLM layer by checking ContentType
	}

	// Store image data for LLM layer
	convCtx.ImageData = &memory.ImageContent{
		Base64:    b64Data,
		MediaType: "image/jpeg",
		Caption:   caption,
	}
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
```

- [ ] **Step 2: Add ImageContent to memory types and update LLM client for image support**

Add to `internal/memory/manager.go`:

```go
// Add ImageContent struct and ImageData field to ConversationContext

type ImageContent struct {
	Base64    string
	MediaType string
	Caption   string
}

// Update ConversationContext
type ConversationContext struct {
	Facts          []Fact
	Summaries      []Summary
	RecentMessages []Message
	ImageData      *ImageContent
}
```

Update `internal/llm/client.go` `BuildMessages` to handle image content:

```go
func BuildMessages(convCtx *memory.ConversationContext) (string, []anthropic.MessageParam) {
	var sb strings.Builder
	sb.WriteString("You are a helpful personal assistant. Be concise and direct.\n\n")

	if len(convCtx.Facts) > 0 {
		sb.WriteString("## Things I know about the user\n")
		for _, f := range convCtx.Facts {
			sb.WriteString("- ")
			sb.WriteString(f.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(convCtx.Summaries) > 0 {
		sb.WriteString("## Previous conversation summaries\n")
		for _, s := range convCtx.Summaries {
			sb.WriteString("- ")
			sb.WriteString(s.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	messages := make([]anthropic.MessageParam, 0, len(convCtx.RecentMessages))
	for i, msg := range convCtx.RecentMessages {
		isLastMessage := i == len(convCtx.RecentMessages)-1

		switch msg.Role {
		case "user":
			if isLastMessage && convCtx.ImageData != nil {
				// Build image + text message
				messages = append(messages, anthropic.NewUserMessage(
					anthropic.NewImageBlockBase64(
						convCtx.ImageData.MediaType,
						convCtx.ImageData.Base64,
					),
					anthropic.NewTextBlock(convCtx.ImageData.Caption),
				))
			} else {
				messages = append(messages, anthropic.NewUserMessage(
					anthropic.NewTextBlock(msg.Content),
				))
			}
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(
				anthropic.NewTextBlock(msg.Content),
			))
		}
	}

	return sb.String(), messages
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/chat/ internal/memory/manager.go internal/llm/client.go
git commit -m "feat: add chat service with image support and orchestration"
```

---

### Task 10: Main Entry Point

**Files:**
- Create: `cmd/bot/main.go`

- [ ] **Step 1: Write implementation**

```go
// cmd/bot/main.go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/dolphin836/bot/internal/bot"
	"github.com/dolphin836/bot/internal/chat"
	"github.com/dolphin836/bot/internal/config"
	"github.com/dolphin836/bot/internal/llm"
	"github.com/dolphin836/bot/internal/memory"
	"github.com/dolphin836/bot/internal/tools"
	tgbot "github.com/go-telegram/bot"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfgPath := "config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("load_config", "error", err)
		os.Exit(1)
	}

	if cfg.Telegram.Token == "" {
		slog.Error("TELEGRAM_BOT_TOKEN is required")
		os.Exit(1)
	}
	if cfg.Anthropic.APIKey == "" {
		slog.Error("ANTHROPIC_API_KEY is required")
		os.Exit(1)
	}

	// Init SQLite
	store, err := memory.NewStore(cfg.DB.Path)
	if err != nil {
		slog.Error("init_db", "error", err)
		os.Exit(1)
	}

	// Init tool registry
	registry := tools.NewRegistry()

	// Init LLM client
	llmClient := llm.NewClient(cfg.Anthropic.APIKey, cfg.Anthropic.Model, registry)

	// Init compressor
	compressor := llm.NewCompressor(cfg.Anthropic.APIKey, cfg.Anthropic.Model)

	// Init memory manager
	memMgr := memory.NewManager(store, compressor, memory.ManagerConfig{
		RecentLimit:       cfg.Memory.RecentLimit,
		SummaryMaxAgeDays: cfg.Memory.SummaryMaxAgeDays,
	})

	// Init chat service
	chatSvc := chat.NewService(memMgr, llmClient)

	// Init bot handler
	handler := bot.NewHandler(cfg.Telegram.OwnerID, chatSvc)

	// Start Telegram bot
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	b, err := tgbot.New(cfg.Telegram.Token, tgbot.WithDefaultHandler(handler.Handle))
	if err != nil {
		slog.Error("init_bot", "error", err)
		os.Exit(1)
	}

	slog.Info("bot_started", "owner_id", cfg.Telegram.OwnerID)
	b.Start(ctx)
}
```

- [ ] **Step 2: Ensure data directory is created**

Add to `internal/memory/store.go` before opening DB:

```go
func NewStore(dbPath string) (*Store, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." && dir != ":memory:" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}
	// ... rest of existing code
}
```

Add imports `"os"`, `"fmt"`, `"path/filepath"` to store.go.

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/bot/`
Expected: Build succeeds

- [ ] **Step 4: Run all tests**

Run: `go test ./... -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/bot/main.go internal/memory/store.go
git commit -m "feat: add main entry point wiring all components together"
```

---

### Task 11: Update CLAUDE.md and Final Cleanup

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md with actual architecture**

Update the CLAUDE.md to reflect the final implemented structure, removing "Target" label from architecture.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 3: Run linter**

Run: `golangci-lint run ./...`
Expected: No issues (or only minor ones to fix)

- [ ] **Step 4: Final commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with final architecture"
```

- [ ] **Step 5: Push to remote**

```bash
git push origin main
```
