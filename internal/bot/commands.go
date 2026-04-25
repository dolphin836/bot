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
