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
	"github.com/dolphin836/bot/internal/photos"
	"github.com/dolphin836/bot/internal/tools"
	"github.com/dolphin836/bot/internal/tts"
	"github.com/dolphin836/bot/internal/vlog"
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
	registry.Register(tools.NewWeatherTool(tools.WeatherConfig{
		DefaultCity:      cfg.Weather.DefaultCity,
		DefaultLatitude:  cfg.Weather.DefaultLatitude,
		DefaultLongitude: cfg.Weather.DefaultLongitude,
	}))
	registry.Register(tools.NewPhotosTool(store))

	llmClient := llm.NewClient(cfg.Anthropic.APIKey, cfg.Anthropic.Model, cfg.Anthropic.Persona, registry)
	compressor := llm.NewCompressor(cfg.Anthropic.APIKey, cfg.Anthropic.Model)

	memMgr := memory.NewManager(store, compressor, memory.ManagerConfig{
		RecentLimit:       cfg.Memory.RecentLimit,
		SummaryMaxAgeDays: cfg.Memory.SummaryMaxAgeDays,
	})

	ttsSvc := tts.NewService(cfg.TTS.Voice, cfg.TTS.Enabled)

	var scanner *photos.Scanner
	if cfg.Photos.Dir != "" {
		scanner = photos.NewScanner(store, cfg.Anthropic.APIKey, cfg.Anthropic.Model, cfg.Photos.Dir)
	}

	var vlogGen *vlog.Generator
	if cfg.Vlog.Enabled && cfg.Vlog.MediaDir != "" {
		vlogGen = vlog.NewGenerator(vlog.Config{
			MediaDir: cfg.Vlog.MediaDir,
			BGMPath:  cfg.Vlog.BGMPath,
			APIKey:   cfg.Anthropic.APIKey,
			Model:    cfg.Anthropic.Model,
			Persona:  cfg.Anthropic.Persona,
			TTSSvc:   ttsSvc,
			Store:    store,
		})
	}

	chatSvc := chat.NewService(memMgr, llmClient)

	// Handler and vlog scheduler need the bot instance, so create handler first
	// then set up scheduler after bot is created
	handler := bothandler.NewHandler(cfg.Telegram.OwnerID, chatSvc, ttsSvc, scanner, cfg.Vlog.MediaDir, nil)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	b, err := tgbot.New(cfg.Telegram.Token, tgbot.WithDefaultHandler(handler.Handle))
	if err != nil {
		slog.Error("init_bot", "error", err)
		os.Exit(1)
	}

	// Set up vlog scheduler after bot is created
	if vlogGen != nil {
		vlogSched := vlog.NewScheduler(vlogGen, b, cfg.Telegram.OwnerID, cfg.Vlog.ScheduleHour, cfg.Vlog.MinItems)
		handler.SetVlogScheduler(vlogSched)
		go vlogSched.Start(ctx)
	}

	slog.Info("bot_started", "owner_id", cfg.Telegram.OwnerID)
	b.Start(ctx)
}
