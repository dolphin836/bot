# Telegram Personal Assistant Bot — Design Spec

## Overview

Go-based personal assistant Agent. Single user, single perpetual conversation. Communicates via Telegram, powered by Claude API with streaming responses and tool use support.

## Tech Stack

| Component | Package |
|---|---|
| Telegram | `github.com/go-telegram/bot` |
| Claude SDK | `github.com/anthropics/anthropic-sdk-go` |
| SQLite | `modernc.org/sqlite` |
| ORM | `gorm.io/gorm` + `gorm.io/driver/sqlite` |
| Config | `github.com/spf13/viper` |
| Logging | `log/slog` |

## Project Structure

```
cmd/bot/main.go              — entry point, config loading, wiring
internal/
  config/config.go           — config struct and loading (viper, env+yaml)
  bot/
    handler.go               — Telegram update dispatch, user ID auth
    commands.go              — /clear /help /facts /forget command handlers
  chat/
    service.go               — main message processing, orchestrates memory + llm
  memory/
    store.go                 — SQLite persistence layer (GORM)
    manager.go               — three-tier memory management (compress, extract, assemble context)
  llm/
    client.go                — Claude API wrapper, streaming output
    tools.go                 — tool use dispatch, tool_result loop
  tools/
    registry.go              — tool registry (interface + register/lookup)
```

## Data Model (SQLite)

Single user, single perpetual conversation. No user_id or conversation_id needed.

### messages
| Column | Type | Description |
|---|---|---|
| id | INTEGER PK | Auto increment |
| role | TEXT | user / assistant / tool |
| content | TEXT | Message content |
| content_type | TEXT | text / image / tool_result |
| tokens_in | INTEGER | Input tokens (nullable) |
| tokens_out | INTEGER | Output tokens (nullable) |
| created_at | DATETIME | Timestamp |

### summaries
| Column | Type | Description |
|---|---|---|
| id | INTEGER PK | Auto increment |
| content | TEXT | Compressed summary |
| from_time | DATETIME | Start of summarized period |
| to_time | DATETIME | End of summarized period |
| created_at | DATETIME | Timestamp |

### facts
| Column | Type | Description |
|---|---|---|
| id | INTEGER PK | Auto increment |
| content | TEXT | Key fact or preference |
| category | TEXT | Category tag |
| created_at | DATETIME | Timestamp |
| updated_at | DATETIME | Timestamp |

### tool_calls
| Column | Type | Description |
|---|---|---|
| id | INTEGER PK | Auto increment |
| message_id | INTEGER FK | Reference to messages.id |
| tool_name | TEXT | Tool identifier |
| input | TEXT (JSON) | Tool input |
| output | TEXT (JSON) | Tool output |
| status | TEXT | pending / success / error |
| created_at | DATETIME | Timestamp |

## Three-Tier Memory

| Tier | Storage | Sent to Claude | Trigger |
|---|---|---|---|
| Recent | Last N messages from `messages` table | Full messages in conversation | Every request |
| Mid-term | `summaries` table | Injected into system prompt | Auto-compress when recent messages exceed window |
| Long-term | `facts` table | Injected into system prompt | Extract key facts during compression |

### Compression Flow

When recent messages exceed `recent_limit` (default 50 rounds):
1. Take oldest messages beyond the window
2. Send to Claude: "Summarize this conversation segment"
3. Store summary in `summaries` table
4. Send to Claude: "Extract key facts, preferences, decisions from this segment"
5. Upsert into `facts` table (deduplicate by content similarity)
6. Delete compressed messages from `messages` table

### Context Assembly (per request)

```
System Prompt:
  - Base persona/instructions
  - All facts (long-term)
  - Recent summaries (mid-term, last N days)
Messages:
  - Recent messages from messages table (last N rounds)
  - Current user message
```

## Message Processing Flow

```
Telegram message/image
  → bot.Handler: verify owner user ID (reject others silently)
  → chat.Service:
    1. Save user message to messages table
    2. memory.Manager assembles context:
       - system prompt + facts + recent summaries + recent messages
    3. llm.Client streaming call to Claude
    4. Stream callback: edit Telegram message every ~1 second (throttle)
    5. If tool_use → tools.Registry.Execute → append tool_result → continue streaming
    6. Save assistant message on completion
    7. Check if window exceeded → trigger compression
```

## Streaming + Telegram Throttle

- Claude streams tokens via SSE
- Accumulate tokens in buffer
- Edit Telegram message at most once per second (Telegram rate limit)
- On completion, final edit with complete response
- Support Markdown formatting in Telegram (MarkdownV2 parse mode)

## Tool Use Architecture

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (string, error)
}

type Registry struct {
    tools map[string]Tool
}
```

- Tools self-register at startup
- llm.Client passes registered tools to Claude API
- When Claude returns tool_use, dispatch to Registry
- Feed tool_result back, continue until Claude returns text
- Specific tools added incrementally in later iterations

## Commands

| Command | Action |
|---|---|
| `/clear` | Delete all messages, summaries, facts. Fresh start. |
| `/help` | List available commands |
| `/facts` | Display all stored long-term facts |
| `/forget <keyword>` | Delete matching facts from long-term memory |

## Access Control

Single user ID whitelist. Configured in `config.yaml` as `telegram.owner_id`. All updates from other users are silently ignored.

## Multimodal Support

- Images sent in Telegram are downloaded via Bot API
- Converted to base64, sent to Claude as image content blocks
- Claude vision analyzes and responds as text

## Configuration (config.yaml)

```yaml
telegram:
  token: ${TELEGRAM_BOT_TOKEN}
  owner_id: 123456789

anthropic:
  api_key: ${ANTHROPIC_API_KEY}
  model: claude-sonnet-4-20250514

memory:
  recent_limit: 50
  summary_max_age_days: 30

db:
  path: ./data/bot.db
```

Secrets loaded from environment variables, non-secret defaults in yaml.
