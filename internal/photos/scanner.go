package photos

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
)

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".heic": true, ".webp": true, ".gif": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
}

type Scanner struct {
	store  *memory.Store
	client anthropic.Client
	model  string
	dir    string
}

func NewScanner(store *memory.Store, apiKey string, model string, dir string) *Scanner {
	return &Scanner{
		store:  store,
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
		dir:    dir,
	}
}

func (s *Scanner) Store() *memory.Store {
	return s.store
}

// ScanDir scans the photo directory and registers all files in the index.
// Does not generate descriptions — call IndexUnprocessed for that.
func (s *Scanner) ScanDir() (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, fmt.Errorf("read dir %s: %w", s.dir, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		fileType := ""
		if imageExts[ext] {
			fileType = "image"
		} else if videoExts[ext] {
			fileType = "video"
		} else {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		err = s.store.UpsertPhoto(&memory.PhotoIndex{
			Filename: entry.Name(),
			FilePath: filepath.Join(s.dir, entry.Name()),
			FileType: fileType,
			FileSize: info.Size(),
			ModTime:  info.ModTime(),
		})
		if err != nil {
			slog.Error("upsert_photo", "file", entry.Name(), "error", err)
			continue
		}
		count++
	}

	return count, nil
}

// IndexUnprocessed generates descriptions for photos that don't have one yet.
// Returns the number of photos processed.
func (s *Scanner) IndexUnprocessed(ctx context.Context, onProgress func(current, total int)) (int, error) {
	photos, err := s.store.GetUnindexedPhotos()
	if err != nil {
		return 0, err
	}

	total := len(photos)
	processed := 0

	for i, photo := range photos {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}

		if onProgress != nil {
			onProgress(i+1, total)
		}

		desc, err := s.describePhoto(ctx, photo)
		if err != nil {
			slog.Error("describe_photo", "file", photo.Filename, "error", err)
			continue
		}

		photo.Description = desc
		photo.IndexedAt = time.Now()
		if err := s.store.UpsertPhoto(&photo); err != nil {
			slog.Error("save_description", "file", photo.Filename, "error", err)
			continue
		}

		processed++
		slog.Info("indexed_photo", "file", photo.Filename, "progress", fmt.Sprintf("%d/%d", i+1, total))
	}

	return processed, nil
}

func (s *Scanner) describePhoto(ctx context.Context, photo memory.PhotoIndex) (string, error) {
	switch photo.FileType {
	case "image":
		return s.describeImage(ctx, photo)
	case "video":
		return s.describeVideo(ctx, photo)
	default:
		return "", fmt.Errorf("unknown file type: %s", photo.FileType)
	}
}

func (s *Scanner) describeImage(ctx context.Context, photo memory.PhotoIndex) (string, error) {
	imgData, mediaType, err := readImageAsBase64(photo.FilePath)
	if err != nil {
		return "", err
	}

	msg, err := s.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:    anthropic.Model(s.model),
		MaxTokens: 300,
		System: []anthropic.TextBlockParam{
			{Text: "Describe this photo concisely in Chinese. Include: what's in the photo (people, objects, places, activities), " +
				"the mood/atmosphere, notable details, and estimated season/time of day if visible. " +
				"Write 2-3 sentences as if recalling a personal memory."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64(mediaType, imgData),
				anthropic.NewTextBlock("请描述这张照片"),
			),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude vision: %w", err)
	}

	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			return text.Text, nil
		}
	}
	return "", fmt.Errorf("no text in response")
}

func (s *Scanner) describeVideo(ctx context.Context, photo memory.PhotoIndex) (string, error) {
	// Extract a thumbnail frame from video using ffmpeg
	tmpFile := filepath.Join(os.TempDir(), "bot_thumb_"+photo.Filename+".jpg")
	defer os.Remove(tmpFile)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", photo.FilePath,
		"-ss", "00:00:01",
		"-vframes", "1",
		"-y",
		tmpFile,
	)
	if err := cmd.Run(); err != nil {
		// If ffmpeg fails, just describe by filename
		return fmt.Sprintf("视频文件: %s (无法提取缩略图)", photo.Filename), nil
	}

	imgData, err := os.ReadFile(tmpFile)
	if err != nil {
		return fmt.Sprintf("视频文件: %s", photo.Filename), nil
	}

	b64 := base64.StdEncoding.EncodeToString(imgData)

	msg, err := s.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:    anthropic.Model(s.model),
		MaxTokens: 300,
		System: []anthropic.TextBlockParam{
			{Text: "This is a frame from a video. Describe what's happening in Chinese. Include: " +
				"the scene, people/objects, activities, and atmosphere. Write 2-3 sentences as if recalling a personal memory."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64("image/jpeg", b64),
				anthropic.NewTextBlock("请描述这个视频片段"),
			),
		},
	})
	if err != nil {
		return fmt.Sprintf("视频文件: %s", photo.Filename), nil
	}

	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			return "[视频] " + text.Text, nil
		}
	}
	return fmt.Sprintf("视频文件: %s", photo.Filename), nil
}

func readImageAsBase64(filePath string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	// HEIC needs conversion
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

	// Try sips (macOS built-in)
	cmd := exec.Command("sips", "-s", "format", "jpeg", filePath, "--out", tmpFile)
	if err := cmd.Run(); err != nil {
		// Try ffmpeg as fallback
		cmd = exec.Command("ffmpeg", "-i", filePath, "-y", tmpFile)
		if err := cmd.Run(); err != nil {
			return "", "", fmt.Errorf("convert HEIC: sips and ffmpeg both failed: %w", err)
		}
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return "", "", err
	}

	return base64.StdEncoding.EncodeToString(data), "image/jpeg", nil
}
