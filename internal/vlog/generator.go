package vlog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/dolphin836/bot/internal/memory"
	"github.com/dolphin836/bot/internal/tts"
)

type Config struct {
	MediaDir string
	BGMPath  string
	APIKey   string
	Model    string
	Persona  string
	TTSSvc   *tts.Service
	Store    *memory.Store
}

type Generator struct {
	cfg    Config
	client anthropic.Client
}

func NewGenerator(cfg Config) *Generator {
	return &Generator{
		cfg:    cfg,
		client: anthropic.NewClient(option.WithAPIKey(cfg.APIKey)),
	}
}

type DailyContent struct {
	Date     string
	Messages []memory.Message
	Photos   []string // file paths
	Videos   []string // file paths
}

// CollectDaily gathers all content for a given date.
func (g *Generator) CollectDaily(date string) (*DailyContent, error) {
	content := &DailyContent{Date: date}

	// Parse date
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("parse date: %w", err)
	}
	dayStart := t
	dayEnd := t.Add(24 * time.Hour)

	// Get messages for the day
	var msgs []memory.Message
	g.cfg.Store.DB().Where("created_at >= ? AND created_at < ? AND role IN ?",
		dayStart, dayEnd, []string{"user", "assistant"}).
		Order("id asc").Find(&msgs)
	content.Messages = msgs

	// Get media files for the day
	dayDir := filepath.Join(g.cfg.MediaDir, date)
	entries, err := os.ReadDir(dayDir)
	if err != nil {
		// No media directory for this day, that's OK
		return content, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dayDir, entry.Name())
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".heic", ".webp":
			content.Photos = append(content.Photos, path)
		case ".mp4", ".mov", ".avi":
			content.Videos = append(content.Videos, path)
		}
	}

	return content, nil
}

// HasEnoughContent checks if there's enough content to generate a vlog.
func (dc *DailyContent) HasEnoughContent(minItems int) bool {
	return len(dc.Photos)+len(dc.Videos) >= minItems
}

// Generate creates a vlog video for the given daily content.
// Returns the path to the generated video file.
func (g *Generator) Generate(ctx context.Context, content *DailyContent) (string, error) {
	slog.Info("vlog_generate_start", "date", content.Date,
		"photos", len(content.Photos), "videos", len(content.Videos), "messages", len(content.Messages))

	// Step 1: Generate narration script
	script, err := g.generateScript(ctx, content)
	if err != nil {
		return "", fmt.Errorf("generate script: %w", err)
	}
	slog.Info("vlog_script_generated", "length", len(script))

	// Step 2: Generate narration audio via TTS
	narrationPath := filepath.Join(g.cfg.MediaDir, content.Date, "narration.mp3")
	audioData, err := g.cfg.TTSSvc.Synthesize(ctx, script)
	if err != nil {
		return "", fmt.Errorf("tts synthesize: %w", err)
	}
	if err := os.WriteFile(narrationPath, audioData, 0644); err != nil {
		return "", fmt.Errorf("write narration: %w", err)
	}

	// Step 3: Compose video with FFmpeg
	outputPath := filepath.Join(g.cfg.MediaDir, content.Date, "vlog.mp4")
	if err := g.compose(ctx, content, narrationPath, outputPath); err != nil {
		return "", fmt.Errorf("compose video: %w", err)
	}

	slog.Info("vlog_generated", "path", outputPath)
	return outputPath, nil
}

func (g *Generator) generateScript(ctx context.Context, content *DailyContent) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("今天是%s，以下是今天的内容：\n\n", content.Date))

	if len(content.Messages) > 0 {
		sb.WriteString("## 今天的对话\n")
		for _, msg := range content.Messages {
			if msg.Role == "user" {
				sb.WriteString(fmt.Sprintf("他说: %s\n", msg.Content))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("今天收到了 %d 张照片和 %d 个视频。\n", len(content.Photos), len(content.Videos)))

	persona := g.cfg.Persona
	if persona == "" {
		persona = "你是一个温柔的女生"
	}

	msg, err := g.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:    anthropic.Model(g.cfg.Model),
		MaxTokens: 500,
		System: []anthropic.TextBlockParam{
			{Text: persona + "\n\n" +
				"现在你要为今天写一段 vlog 旁白。要求：\n" +
				"- 用第一人称，就像在录 vlog 自言自语\n" +
				"- 温暖、自然、有感情，像在和好朋友分享日常\n" +
				"- 提到今天的聊天内容和收到的照片/视频\n" +
				"- 长度控制在 100-200 字\n" +
				"- 不要加标题或标点以外的格式符号\n" +
				"- 以一句温暖的结尾收束"},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(sb.String())),
		},
	})
	if err != nil {
		return "", err
	}

	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			return text.Text, nil
		}
	}
	return "", fmt.Errorf("no text in response")
}

func (g *Generator) compose(ctx context.Context, content *DailyContent, narrationPath string, outputPath string) error {
	// Build a list of visual segments (photos + videos)
	var inputs []string
	var filterParts []string
	segIdx := 0

	// Each photo: 5 seconds with Ken Burns (slow zoom) effect
	for _, photo := range content.Photos {
		inputs = append(inputs, "-loop", "1", "-t", "5", "-i", photo)
		// Scale to 1080p, apply slow zoom
		filterParts = append(filterParts,
			fmt.Sprintf("[%d:v]scale=1920:1080:force_original_aspect_ratio=increase,crop=1920:1080,zoompan=z='min(zoom+0.001,1.2)':d=150:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=1920x1080,setsar=1[v%d]",
				segIdx, segIdx))
		segIdx++
	}

	// Each video: first 10 seconds, scaled to 1080p
	for _, video := range content.Videos {
		inputs = append(inputs, "-t", "10", "-i", video)
		filterParts = append(filterParts,
			fmt.Sprintf("[%d:v]scale=1920:1080:force_original_aspect_ratio=increase,crop=1920:1080,setsar=1[v%d]",
				segIdx, segIdx))
		segIdx++
	}

	if segIdx == 0 {
		return fmt.Errorf("no visual content to compose")
	}

	// Narration audio input
	narrationIdx := segIdx
	inputs = append(inputs, "-i", narrationPath)
	segIdx++

	// BGM audio input (optional)
	bgmIdx := -1
	hasBGM := false
	if g.cfg.BGMPath != "" {
		if _, err := os.Stat(g.cfg.BGMPath); err == nil {
			bgmIdx = segIdx
			inputs = append(inputs, "-i", g.cfg.BGMPath)
			hasBGM = true
			segIdx++
		}
	}

	// Concat all video segments
	var concatInputs string
	for i := 0; i < narrationIdx; i++ {
		concatInputs += fmt.Sprintf("[v%d]", i)
	}
	filterParts = append(filterParts,
		fmt.Sprintf("%sconcat=n=%d:v=1:a=0[vout]", concatInputs, narrationIdx))

	// Audio mixing: narration + BGM (BGM at lower volume)
	audioFilter := fmt.Sprintf("[%d:a]aformat=sample_rates=44100:channel_layouts=stereo[narr]", narrationIdx)
	filterParts = append(filterParts, audioFilter)

	if hasBGM {
		// BGM looped, volume reduced, mixed with narration
		filterParts = append(filterParts,
			fmt.Sprintf("[%d:a]aloop=-1:size=2e9,aformat=sample_rates=44100:channel_layouts=stereo,volume=0.15[bgm]", bgmIdx))
		filterParts = append(filterParts,
			"[narr][bgm]amix=inputs=2:duration=first:dropout_transition=2[aout]")
	} else {
		filterParts = append(filterParts, "[narr]acopy[aout]")
	}

	filterComplex := strings.Join(filterParts, ";")

	args := []string{}
	args = append(args, inputs...)
	args = append(args,
		"-filter_complex", filterComplex,
		"-map", "[vout]",
		"-map", "[aout]",
		"-c:v", "libx264", "-preset", "fast", "-crf", "23",
		"-c:a", "aac", "-b:a", "128k",
		"-shortest",
		"-y", outputPath,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}

	return nil
}
