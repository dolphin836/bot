package vlog

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
			s.run(ctx)
		}
	}
}

// RunNow triggers vlog generation for today immediately.
func (s *Scheduler) RunNow(ctx context.Context) {
	s.run(ctx)
}

func (s *Scheduler) run(ctx context.Context) {
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

	// Send the video
	videoData, err := os.ReadFile(videoPath)
	if err != nil {
		slog.Error("vlog_read", "error", err)
		return
	}

	_, err = s.bot.SendVideo(ctx, &bot.SendVideoParams{
		ChatID: s.ownerID,
		Video: &models.InputFileUpload{
			Filename: "vlog_" + date + ".mp4",
			Data:     bytes.NewReader(videoData),
		},
		Caption: "今天的 vlog 来啦~",
	})
	if err != nil {
		slog.Error("vlog_send", "error", err)
	}
}
