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
	case "/help", "/start":
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
	case "/scan":
		h.cmdScan(ctx, b, msg)
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
/forget <keyword> — Delete facts matching keyword
/scan — Scan and index local photo album`

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   text,
	})
}

func (h *Handler) cmdClear(ctx context.Context, b *bot.Bot, msg *models.Message) {
	if err := h.chatSvc.ClearAll(); err != nil {
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

	if err := h.chatSvc.ForgetFacts(keyword); err != nil {
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

func (h *Handler) cmdScan(ctx context.Context, b *bot.Bot, msg *models.Message) {
	if h.scanner == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Photo scanning is not configured.",
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "开始扫描相册目录...",
	})

	fileCount, err := h.scanner.ScanDir()
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   fmt.Sprintf("扫描出错: %v", err),
		})
		return
	}

	total, _ := h.scanner.Store().PhotoCount()
	indexed, _ := h.scanner.Store().IndexedPhotoCount()
	unindexed := total - indexed

	if unindexed == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   fmt.Sprintf("扫描完成，共 %d 个文件，全部已建立索引。", total),
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   fmt.Sprintf("发现 %d 个文件（新增 %d），开始用 AI 分析生成描述...\n这可能需要几分钟，请耐心等待~", total, unindexed),
	})

	_ = fileCount

	processed, err := h.scanner.IndexUnprocessed(ctx, func(current, total int) {
		if current%5 == 0 || current == total {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text:   fmt.Sprintf("正在分析... %d/%d", current, total),
			})
		}
	})

	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   fmt.Sprintf("分析过程中出错: %v\n已完成 %d 张", err, processed),
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   fmt.Sprintf("全部完成！成功分析了 %d 张照片/视频，现在可以问我关于你相册里的事情了~", processed),
	})
}
