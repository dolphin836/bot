package bot

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// saveMedia downloads and saves a file to the daily media directory.
// Returns the saved file path.
func saveMedia(mediaDir string, downloadURL string, filename string) (string, error) {
	dayDir := filepath.Join(mediaDir, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dayDir, 0755); err != nil {
		return "", fmt.Errorf("create media dir: %w", err)
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	savePath := filepath.Join(dayDir, filename)
	f, err := os.Create(savePath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return savePath, nil
}
