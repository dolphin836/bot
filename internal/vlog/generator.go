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
	Photos   []string
	Videos   []string
}

func (g *Generator) CollectDaily(date string) (*DailyContent, error) {
	content := &DailyContent{Date: date}

	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("parse date: %w", err)
	}
	dayStart := t
	dayEnd := t.Add(24 * time.Hour)

	var msgs []memory.Message
	g.cfg.Store.DB().Where("created_at >= ? AND created_at < ? AND role IN ?",
		dayStart, dayEnd, []string{"user", "assistant"}).
		Order("id asc").Find(&msgs)
	content.Messages = msgs

	dayDir := filepath.Join(g.cfg.MediaDir, date)
	entries, err := os.ReadDir(dayDir)
	if err != nil {
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

func (dc *DailyContent) HasEnoughContent(minItems int) bool {
	return len(dc.Photos)+len(dc.Videos) >= minItems
}

func (g *Generator) Generate(ctx context.Context, content *DailyContent) (string, error) {
	slog.Info("vlog_generate_start", "date", content.Date,
		"photos", len(content.Photos), "videos", len(content.Videos), "messages", len(content.Messages))

	// Step 1: Generate narration script
	script, err := g.generateScript(ctx, content)
	if err != nil {
		return "", fmt.Errorf("generate script: %w", err)
	}
	slog.Info("vlog_script_generated", "length", len(script))

	// Step 2: Generate narration audio
	dayDir := filepath.Join(g.cfg.MediaDir, content.Date)
	os.MkdirAll(dayDir, 0755)

	narrationPath := filepath.Join(dayDir, "narration.mp3")
	audioData, err := g.cfg.TTSSvc.Synthesize(ctx, script)
	if err != nil {
		return "", fmt.Errorf("tts synthesize: %w", err)
	}
	if err := os.WriteFile(narrationPath, audioData, 0644); err != nil {
		return "", fmt.Errorf("write narration: %w", err)
	}

	// Get narration duration to calculate per-segment timing
	narrationDur, err := getAudioDuration(narrationPath)
	if err != nil {
		narrationDur = 30.0 // fallback
	}

	// Step 3: Compose video
	outputPath := filepath.Join(dayDir, "vlog.mp4")
	if err := g.compose(ctx, content, narrationPath, narrationDur, outputPath); err != nil {
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
				"- 用第一人称，就像在录 vlog 对着镜头说话\n" +
				"- 温暖、自然、有感情，像在和最好的朋友分享日常\n" +
				"- 根据聊天内容提到今天发生的事情和心情\n" +
				"- 长度控制在 150-250 字（约 30-60 秒朗读时长）\n" +
				"- 不要加标题、emoji 或格式符号\n" +
				"- 开头自然引入，结尾温暖收束"},
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

func getAudioDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	var dur float64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &dur)
	return dur, nil
}

func (g *Generator) compose(ctx context.Context, content *DailyContent, narrationPath string, narrationDur float64, outputPath string) error {
	totalMedia := len(content.Photos) + len(content.Videos)
	if totalMedia == 0 {
		return fmt.Errorf("no media to compose")
	}

	// Calculate per-segment duration based on narration length
	// Photos get equal share, videos use their own length (capped)
	maxVideoDur := 8.0
	photoDur := narrationDur / float64(totalMedia)
	if photoDur < 3.0 {
		photoDur = 3.0
	}
	if photoDur > 8.0 {
		photoDur = 8.0
	}

	// Strategy: create individual segment clips, then concat with crossfade
	segmentDir := filepath.Join(filepath.Dir(outputPath), "segments")
	os.MkdirAll(segmentDir, 0755)
	defer os.RemoveAll(segmentDir)

	var segmentPaths []string
	segIdx := 0

	// Process photos — Ken Burns with slow zoom + pan
	for _, photo := range content.Photos {
		segPath := filepath.Join(segmentDir, fmt.Sprintf("seg_%03d.mp4", segIdx))

		// Alternate between zoom-in and zoom-out for variety
		var zoompan string
		switch segIdx % 3 {
		case 0: // Slow zoom in
			zoompan = fmt.Sprintf("zoompan=z='min(zoom+0.0008,1.3)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=1080x1920:fps=30",
				int(photoDur*30))
		case 1: // Pan left to right
			zoompan = fmt.Sprintf("zoompan=z='1.15':d=%d:x='if(eq(on,1),0,x+2)':y='ih/4':s=1080x1920:fps=30",
				int(photoDur*30))
		case 2: // Slow zoom out
			zoompan = fmt.Sprintf("zoompan=z='if(eq(on,1),1.3,max(zoom-0.0008,1.0))':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=1080x1920:fps=30",
				int(photoDur*30))
		}

		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-loop", "1", "-i", photo,
			"-vf", fmt.Sprintf("scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black,%s", zoompan),
			"-t", fmt.Sprintf("%.1f", photoDur),
			"-c:v", "libx264", "-preset", "fast", "-crf", "20",
			"-pix_fmt", "yuv420p",
			"-an", "-y", segPath,
		)
		if err := cmd.Run(); err != nil {
			slog.Error("ffmpeg_photo_segment", "file", photo, "error", err)
			continue
		}
		segmentPaths = append(segmentPaths, segPath)
		segIdx++
	}

	// Process videos — trim, scale to match
	for _, video := range content.Videos {
		segPath := filepath.Join(segmentDir, fmt.Sprintf("seg_%03d.mp4", segIdx))

		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-i", video,
			"-t", fmt.Sprintf("%.1f", maxVideoDur),
			"-vf", "scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black",
			"-c:v", "libx264", "-preset", "fast", "-crf", "20",
			"-pix_fmt", "yuv420p",
			"-an", "-y", segPath,
		)
		if err := cmd.Run(); err != nil {
			slog.Error("ffmpeg_video_segment", "file", video, "error", err)
			continue
		}
		segmentPaths = append(segmentPaths, segPath)
		segIdx++
	}

	if len(segmentPaths) == 0 {
		return fmt.Errorf("no segments created")
	}

	// Create concat list file
	concatList := filepath.Join(segmentDir, "concat.txt")
	var listContent strings.Builder
	for _, seg := range segmentPaths {
		listContent.WriteString(fmt.Sprintf("file '%s'\n", seg))
	}
	if err := os.WriteFile(concatList, []byte(listContent.String()), 0644); err != nil {
		return fmt.Errorf("write concat list: %w", err)
	}

	// Concat segments with crossfade would be complex, use simple concat for reliability
	// Then mix narration + BGM

	concatVideo := filepath.Join(segmentDir, "concat.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "concat", "-safe", "0",
		"-i", concatList,
		"-c:v", "libx264", "-preset", "fast", "-crf", "20",
		"-pix_fmt", "yuv420p",
		"-an", "-y", concatVideo,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg concat: %w", err)
	}

	// Final mix: video + narration + BGM
	hasBGM := false
	if g.cfg.BGMPath != "" {
		if info, err := os.Stat(g.cfg.BGMPath); err == nil && info.Size() > 1000 {
			hasBGM = true
		}
	}

	if hasBGM {
		cmd = exec.CommandContext(ctx, "ffmpeg",
			"-i", concatVideo,
			"-i", narrationPath,
			"-i", g.cfg.BGMPath,
			"-filter_complex",
			"[1:a]aformat=sample_rates=44100:channel_layouts=stereo,adelay=500|500[narr];"+
				"[2:a]aloop=-1:size=2e9,aformat=sample_rates=44100:channel_layouts=stereo,volume=0.12[bgm];"+
				"[narr][bgm]amix=inputs=2:duration=first:dropout_transition=3[aout]",
			"-map", "0:v", "-map", "[aout]",
			"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
			"-shortest",
			"-y", outputPath,
		)
	} else {
		cmd = exec.CommandContext(ctx, "ffmpeg",
			"-i", concatVideo,
			"-i", narrationPath,
			"-map", "0:v", "-map", "1:a",
			"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
			"-shortest",
			"-y", outputPath,
		)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg final mix: %w", err)
	}

	return nil
}
