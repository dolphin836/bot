package bot

import (
	"bytes"
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
	"github.com/dolphin836/bot/internal/photos"
	"github.com/dolphin836/bot/internal/tts"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type Handler struct {
	ownerID int64
	chatSvc *chat.Service
	ttsSvc  *tts.Service
	scanner *photos.Scanner
	mu      sync.Mutex
}

func NewHandler(ownerID int64, chatSvc *chat.Service, ttsSvc *tts.Service, scanner *photos.Scanner) *Handler {
	return &Handler{
		ownerID: ownerID,
		chatSvc: chatSvc,
		ttsSvc:  ttsSvc,
		scanner: scanner,
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

	if msg.Text != "" && strings.HasPrefix(msg.Text, "/") {
		h.handleCommand(ctx, b, msg)
		return
	}

	if msg.Text != "" {
		h.handleText(ctx, b, msg, true)
		return
	}

	if msg.Voice != nil {
		h.handleVoice(ctx, b, msg)
		return
	}

	if len(msg.Photo) > 0 {
		h.handlePhoto(ctx, b, msg)
		return
	}
}

func (h *Handler) handleText(ctx context.Context, b *bot.Bot, msg *models.Message, replyAsVoice bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "...",
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

	// Final text update
	b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    msg.Chat.ID,
		MessageID: sent.ID,
		Text:      reply,
	})

	// Send voice reply if requested and TTS is enabled
	if replyAsVoice && h.ttsSvc != nil && h.ttsSvc.Enabled() {
		h.sendVoiceReply(ctx, b, msg.Chat.ID, reply)
	}
}

func (h *Handler) handleVoice(ctx context.Context, b *bot.Bot, msg *models.Message) {
	// Voice input requires STT (not yet integrated)
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "我听到了你的语音~ 不过目前还没有接入语音识别服务，你可以先用文字和我聊天，我会用语音回复你哦",
	})
}

func (h *Handler) sendVoiceReply(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	audioData, err := h.ttsSvc.Synthesize(ctx, text)
	if err != nil {
		slog.Error("tts_synthesize", "error", err)
		return
	}

	_, err = b.SendVoice(ctx, &bot.SendVoiceParams{
		ChatID: chatID,
		Voice: &models.InputFileUpload{
			Filename: "voice.mp3",
			Data:     bytes.NewReader(audioData),
		},
	})
	if err != nil {
		slog.Error("send_voice", "error", err)
	}
}

func (h *Handler) handlePhoto(ctx context.Context, b *bot.Bot, msg *models.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

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

	content := b64 + "|||" + caption

	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "...",
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
