package vlog

import (
	"context"
	"encoding/base64"
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

type mediaCaption struct {
	Path    string
	Type    string // "photo" or "video"
	Caption string
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
		"photos", len(content.Photos), "videos", len(content.Videos))

	dayDir := filepath.Join(g.cfg.MediaDir, content.Date)
	os.MkdirAll(dayDir, 0755)

	// Step 1: Generate captions for each media
	captions, err := g.generateCaptions(ctx, content)
	if err != nil {
		return "", fmt.Errorf("generate captions: %w", err)
	}
	slog.Info("vlog_captions_generated", "count", len(captions))

	// Step 2: Compose video segments with subtitles
	outputPath := filepath.Join(dayDir, "vlog.mp4")
	if err := g.compose(ctx, captions, outputPath); err != nil {
		return "", fmt.Errorf("compose video: %w", err)
	}

	slog.Info("vlog_generated", "path", outputPath)
	return outputPath, nil
}

func (g *Generator) generateCaptions(ctx context.Context, content *DailyContent) ([]mediaCaption, error) {
	var result []mediaCaption

	// Caption each photo
	for _, photo := range content.Photos {
		caption, err := g.captionImage(ctx, photo)
		if err != nil {
			slog.Error("caption_photo", "file", photo, "error", err)
			caption = "记录生活的一刻"
		}
		result = append(result, mediaCaption{Path: photo, Type: "photo", Caption: caption})
	}

	// Caption each video (from thumbnail)
	for _, video := range content.Videos {
		caption, err := g.captionVideo(ctx, video)
		if err != nil {
			slog.Error("caption_video", "file", video, "error", err)
			caption = "一段小视频"
		}
		result = append(result, mediaCaption{Path: video, Type: "video", Caption: caption})
	}

	return result, nil
}

func (g *Generator) captionImage(ctx context.Context, photoPath string) (string, error) {
	imgData, mediaType, err := readImageAsBase64(photoPath)
	if err != nil {
		return "", err
	}

	msg, err := g.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:    anthropic.Model(g.cfg.Model),
		MaxTokens: 80,
		System: []anthropic.TextBlockParam{
			{Text: "为这张照片写一句简短的字幕，用中文，第三人称口吻。" +
				"简洁描述画面内容，或发表一点温暖的感想。" +
				"只输出一句话，不超过 20 个字，不加标点以外的符号。"},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64(mediaType, imgData),
				anthropic.NewTextBlock("写字幕"),
			),
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

func (g *Generator) captionVideo(ctx context.Context, videoPath string) (string, error) {
	tmpThumb := filepath.Join(os.TempDir(), fmt.Sprintf("vlog_thumb_%d.jpg", time.Now().UnixNano()))
	defer os.Remove(tmpThumb)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath, "-ss", "00:00:01", "-vframes", "1", "-y", tmpThumb,
	)
	if err := cmd.Run(); err != nil {
		return "一段小视频", nil
	}

	return g.captionImage(ctx, tmpThumb)
}

func (g *Generator) compose(ctx context.Context, captions []mediaCaption, outputPath string) error {
	if len(captions) == 0 {
		return fmt.Errorf("no media to compose")
	}

	segmentDir := filepath.Join(filepath.Dir(outputPath), "segments")
	os.MkdirAll(segmentDir, 0755)
	defer os.RemoveAll(segmentDir)

	// Each segment: 4-6 seconds
	photoDur := 5.0
	videoDur := 8.0

	var segmentPaths []string

	for i, mc := range captions {
		segPath := filepath.Join(segmentDir, fmt.Sprintf("seg_%03d.mp4", i))
		var err error

		if mc.Type == "photo" {
			err = g.composePhotoSegment(ctx, mc, segPath, photoDur, i)
		} else {
			err = g.composeVideoSegment(ctx, mc, segPath, videoDur)
		}

		if err != nil {
			slog.Error("compose_segment", "file", mc.Path, "error", err)
			continue
		}
		segmentPaths = append(segmentPaths, segPath)
	}

	if len(segmentPaths) == 0 {
		return fmt.Errorf("no segments created")
	}

	// Concat all segments
	concatList := filepath.Join(segmentDir, "concat.txt")
	var listContent strings.Builder
	for _, seg := range segmentPaths {
		absSeg, _ := filepath.Abs(seg)
		listContent.WriteString(fmt.Sprintf("file '%s'\n", absSeg))
	}
	if err := os.WriteFile(concatList, []byte(listContent.String()), 0644); err != nil {
		return fmt.Errorf("write concat list: %w", err)
	}

	concatVideo := filepath.Join(segmentDir, "concat.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "concat", "-safe", "0", "-i", concatList,
		"-c:v", "libx264", "-preset", "fast", "-crf", "20",
		"-pix_fmt", "yuv420p", "-an", "-y", concatVideo,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		slog.Error("ffmpeg_concat", "stderr", stderr.String())
		return fmt.Errorf("ffmpeg concat: %w", err)
	}

	// Add BGM
	hasBGM := false
	if g.cfg.BGMPath != "" {
		if info, err := os.Stat(g.cfg.BGMPath); err == nil && info.Size() > 1000 {
			hasBGM = true
		}
	}

	if hasBGM {
		cmd = exec.CommandContext(ctx, "ffmpeg",
			"-i", concatVideo,
			"-i", g.cfg.BGMPath,
			"-filter_complex",
			"[1:a]aloop=-1:size=2e9,aformat=sample_rates=44100:channel_layouts=stereo,volume=0.6[bgm]",
			"-map", "0:v", "-map", "[bgm]",
			"-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
			"-shortest",
			"-y", outputPath,
		)
		var mixStderr strings.Builder
		cmd.Stderr = &mixStderr
		if err := cmd.Run(); err != nil {
			slog.Error("ffmpeg_bgm_mix", "stderr", mixStderr.String())
			return fmt.Errorf("ffmpeg bgm mix: %w", err)
		}
	} else {
		// No BGM, just copy
		cmd = exec.CommandContext(ctx, "ffmpeg",
			"-i", concatVideo,
			"-c:v", "copy", "-an", "-y", outputPath,
		)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ffmpeg copy: %w", err)
		}
	}

	return nil
}

func (g *Generator) composePhotoSegment(ctx context.Context, mc mediaCaption, segPath string, dur float64, idx int) error {
	// Ken Burns effects alternate
	var zoompan string
	frames := int(dur * 30)
	switch idx % 3 {
	case 0:
		zoompan = fmt.Sprintf("zoompan=z='min(zoom+0.0008,1.3)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=1080x1920:fps=30", frames)
	case 1:
		zoompan = fmt.Sprintf("zoompan=z='1.15':d=%d:x='if(eq(on,1),0,x+2)':y='ih/4':s=1080x1920:fps=30", frames)
	case 2:
		zoompan = fmt.Sprintf("zoompan=z='if(eq(on,1),1.3,max(zoom-0.0008,1.0))':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=1080x1920:fps=30", frames)
	}

	// Escape subtitle text for FFmpeg drawtext
	safeCaption := escapeFFmpegText(mc.Caption)

	vf := fmt.Sprintf(
		"scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black,%s,"+
			"drawtext=text='%s':fontsize=36:fontcolor=white:borderw=2:bordercolor=black:"+
			"x=(w-text_w)/2:y=h-120:enable='between(t,0.5,%.1f)'",
		zoompan, safeCaption, dur-0.5,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-loop", "1", "-i", mc.Path,
		"-vf", vf,
		"-t", fmt.Sprintf("%.1f", dur),
		"-c:v", "libx264", "-preset", "fast", "-crf", "20",
		"-pix_fmt", "yuv420p", "-an", "-y", segPath,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		slog.Error("ffmpeg_photo_seg", "stderr", stderr.String())
		return fmt.Errorf("photo segment: %w", err)
	}
	return nil
}

func (g *Generator) composeVideoSegment(ctx context.Context, mc mediaCaption, segPath string, maxDur float64) error {
	safeCaption := escapeFFmpegText(mc.Caption)

	vf := fmt.Sprintf(
		"scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black,"+
			"drawtext=text='%s':fontsize=36:fontcolor=white:borderw=2:bordercolor=black:"+
			"x=(w-text_w)/2:y=h-120:enable='between(t,0.5,%.1f)'",
		safeCaption, maxDur-0.5,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", mc.Path,
		"-t", fmt.Sprintf("%.1f", maxDur),
		"-vf", vf,
		"-c:v", "libx264", "-preset", "fast", "-crf", "20",
		"-pix_fmt", "yuv420p", "-an", "-y", segPath,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		slog.Error("ffmpeg_video_seg", "stderr", stderr.String())
		return fmt.Errorf("video segment: %w", err)
	}
	return nil
}

func escapeFFmpegText(text string) string {
	// FFmpeg drawtext special chars
	r := strings.NewReplacer(
		`\`, `\\\\`,
		`'`, `'\\''`,
		`:`, `\\:`,
		`%`, `%%`,
	)
	return r.Replace(text)
}

func readImageAsBase64(filePath string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	if ext == ".heic" {
		return convertHEICToJPEGBase64(filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", err
	}

	mediaType := "image/jpeg"
	switch ext {
	case ".png":
		mediaType = "image/png"
	case ".gif":
		mediaType = "image/gif"
	case ".webp":
		mediaType = "image/webp"
	}

	return base64.StdEncoding.EncodeToString(data), mediaType, nil
}

func convertHEICToJPEGBase64(filePath string) (string, string, error) {
	tmpFile := filepath.Join(os.TempDir(), "bot_heic_"+filepath.Base(filePath)+".jpg")
	defer os.Remove(tmpFile)

	cmd := exec.Command("sips", "-s", "format", "jpeg", filePath, "--out", tmpFile)
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("ffmpeg", "-i", filePath, "-y", tmpFile)
		if err := cmd.Run(); err != nil {
			return "", "", fmt.Errorf("convert HEIC: %w", err)
		}
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return "", "", err
	}

	return base64.StdEncoding.EncodeToString(data), "image/jpeg", nil
}
