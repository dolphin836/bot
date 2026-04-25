package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	bothandler "github.com/dolphin836/bot/internal/bot"
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
	if cfg.Telegram.OwnerID == 0 {
		slog.Error("telegram.owner_id must be set to your Telegram user ID")
		os.Exit(1)
	}

	store, err := memory.NewStore(cfg.DB.Path)
	if err != nil {
		slog.Error("init_db", "error", err)
		os.Exit(1)
	}

	registry := tools.NewRegistry()

	llmClient := llm.NewClient(cfg.Anthropic.APIKey, cfg.Anthropic.Model, registry)
	compressor := llm.NewCompressor(cfg.Anthropic.APIKey, cfg.Anthropic.Model)

	memMgr := memory.NewManager(store, compressor, memory.ManagerConfig{
		RecentLimit:       cfg.Memory.RecentLimit,
		SummaryMaxAgeDays: cfg.Memory.SummaryMaxAgeDays,
	})

	chatSvc := chat.NewService(memMgr, llmClient)
	handler := bothandler.NewHandler(cfg.Telegram.OwnerID, chatSvc)

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
