package vlog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
)

type Scheduler struct {
	generator *Generator
	bot       *bot.Bot
	ownerID   int64
	hour      int
	minItems  int
}

func NewScheduler(generator *Generator, b *bot.Bot, ownerID int64, hour int, minItems int) *Scheduler {
	return &Scheduler{
		generator: generator,
		bot:       b,
		ownerID:   ownerID,
		hour:      hour,
		minItems:  minItems,
	}
}

// Start begins the daily vlog scheduler. Blocks until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	slog.Info("vlog_scheduler_started", "hour", s.hour)

	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), s.hour, 0, 0, 0, now.Location())
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}
		waitDuration := time.Until(next)
		slog.Info("vlog_next_run", "at", next.Format("2006-01-02 15:04"))

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitDuration):
			s.run(ctx, false)
		}
	}
}

// RunNow triggers vlog generation for today immediately (manual trigger).
func (s *Scheduler) RunNow(ctx context.Context) {
	s.run(ctx, true)
}

func (s *Scheduler) run(ctx context.Context, notify ...bool) {
	shouldNotify := len(notify) > 0 && notify[0]
	date := time.Now().Format("2006-01-02")
	slog.Info("vlog_daily_check", "date", date)

	content, err := s.generator.CollectDaily(date)
	if err != nil {
		slog.Error("vlog_collect", "error", err)
		return
	}

	if !content.HasEnoughContent(s.minItems) {
		slog.Info("vlog_skip", "date", date, "reason", "not enough content",
			"photos", len(content.Photos), "videos", len(content.Videos))
		if shouldNotify {
			s.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: s.ownerID,
				Text:   fmt.Sprintf("今天的素材还不够哦~ 目前有 %d 张照片和 %d 个视频，至少需要 %d 个才能生成 vlog", len(content.Photos), len(content.Videos), s.minItems),
			})
		}
		return
	}

	s.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: s.ownerID,
		Text:   "正在为你生成今天的 vlog~",
	})

	videoPath, err := s.generator.Generate(ctx, content)
	if err != nil {
		slog.Error("vlog_generate", "error", err)
		s.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: s.ownerID,
			Text:   "vlog 生成失败了: " + err.Error(),
		})
		return
	}

	s.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: s.ownerID,
		Text:   fmt.Sprintf("今天的 vlog 已生成~ 保存在 %s", videoPath),
	})
}
