# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Personal assistant Agent that communicates via Telegram and uses the Claude API (Anthropic SDK) as the LLM backend. The primary use case is conversational chat with the owner.

## Tech Stack

- **Language:** Go
- **Telegram:** go-telegram-bot-api or similar Go Telegram library
- **LLM:** Anthropic Claude SDK (`github.com/anthropics/anthropic-sdk-go`)
- **Framework:** Standalone service (not Kratos — this is a personal project, not a team microservice)

## Go Standards

This project follows the Go team standards defined in the `go-team-standards` skill. Key rules that apply here:

- Secrets (Telegram bot token, Anthropic API key) via environment variables or config file, never hardcoded
- Structured logging with `slog`, fields in snake_case
- All errors checked and wrapped with `fmt.Errorf("context: %w", err)`
- No `panic` in business code
- Goroutines must use `context` / `errgroup`
- No commented-out dead code
- Sensitive user data must not leak into logs or prompts

## Architecture (Target)

```
cmd/bot/main.go          — entry point, config loading, wiring
internal/
  bot/                   — Telegram bot setup, update handler, command routing
  chat/                  — conversation session management, message history
  llm/                   — Claude API client wrapper, prompt construction
  config/                — config struct and loading (env / file)
```

- **Bot layer** receives Telegram updates, dispatches to chat/command handlers
- **Chat layer** manages per-user conversation context and message history
- **LLM layer** wraps the Anthropic SDK, handles streaming responses, token management

## Build & Run

```bash
# Build
go build -o bot ./cmd/bot

# Run (requires TELEGRAM_BOT_TOKEN and ANTHROPIC_API_KEY env vars)
TELEGRAM_BOT_TOKEN=xxx ANTHROPIC_API_KEY=xxx ./bot

# Run tests
go test ./...

# Run a single test
go test ./internal/chat -run TestSessionManager

# Lint
golangci-lint run ./...
```

## Design Decisions

- Single-user bot: no multi-tenant auth needed, but access should be restricted to the owner's Telegram user ID
- Conversation history kept in memory (or simple file/SQLite) — no need for PostgreSQL
- Streaming responses preferred for better UX in Telegram (edit message as tokens arrive)
