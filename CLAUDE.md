# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Personal assistant Agent that communicates via Telegram and uses the Claude API (Anthropic SDK) as the LLM backend. Single user, single perpetual conversation with three-tier memory management.

## Tech Stack

- **Language:** Go
- **Telegram:** `github.com/go-telegram/bot`
- **LLM:** `github.com/anthropics/anthropic-sdk-go`
- **Database:** SQLite via `github.com/glebarez/sqlite` (pure Go, no CGO) + GORM
- **Config:** `github.com/spf13/viper` (YAML + env override)
- **Logging:** `log/slog` (stdlib)

## Go Standards

This project follows the Go team standards defined in the `go-team-standards` skill. Key rules:

- Secrets via environment variables (TELEGRAM_BOT_TOKEN, ANTHROPIC_API_KEY), never hardcoded
- Structured logging with `slog`, fields in snake_case
- All errors checked and wrapped with `fmt.Errorf("context: %w", err)`
- No `panic` in business code
- Goroutines must use `context` / `errgroup`

## Architecture

```
cmd/bot/main.go              — entry point, config loading, component wiring
internal/
  config/                    — config struct and viper loading
  bot/
    handler.go               — Telegram update handler, auth gate, message routing
    commands.go              — /help /clear /facts /forget
  chat/
    service.go               — orchestration: message → memory → llm → stream → save
  memory/
    models.go                — GORM models (Message, Summary, Fact, ToolCallRecord)
    store.go                 — SQLite CRUD operations
    manager.go               — three-tier context assembly + compression
  llm/
    client.go                — Claude API streaming + tool use loop
    tools.go                 — Compressor (summary + fact extraction via Claude)
  tools/
    registry.go              — Tool interface + registry
```

**Data flow:** Telegram update → `bot.Handler` (auth) → `chat.Service` → `memory.Manager` (build context) → `llm.Client` (stream + tool loop) → save response → compress if needed

**Three-tier memory:**
- Recent: last N messages (full, in SQLite)
- Mid-term: conversation summaries (auto-generated when window overflows)
- Long-term: key facts/preferences (auto-extracted during compression)

## Build & Run

```bash
go build -o bot ./cmd/bot

TELEGRAM_BOT_TOKEN=xxx ANTHROPIC_API_KEY=xxx ./bot

go test ./...

go test ./internal/memory -run TestCompressTriggersWhenOverLimit

golangci-lint run ./...
```

## Design Decisions

- Single-user bot: access restricted to configured `telegram.owner_id`
- Single perpetual conversation with automatic memory compression (no explicit sessions)
- Streaming responses with 1s throttle on Telegram message edits
- Tool use architecture ready: implement `tools.Tool` interface and register at startup
- Config via `config.yaml` with env var overrides for secrets
