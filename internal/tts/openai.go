package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type OpenAITTS struct {
	APIKey   string
	Voice    string // "alloy", "echo", "fable", "onyx", "nova", "shimmer"
	AudioDir string
}

func (p *OpenAITTS) Generate(ctx context.Context, text string) (string, error) {
	hash := sha256Short(text)
	filename := "tts_" + hash + ".mp3"
	outPath := filepath.Join(p.AudioDir, filename)

	if fileExists(outPath) {
		return filename, nil
	}

	if err := os.MkdirAll(p.AudioDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create audio directory: %w", err)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": "tts-1",
		"input": text,
		"voice": p.Voice,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/audio/speech", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai tts failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai tts error (status %d): %s", resp.StatusCode, string(b))
	}

	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", err
	}

	return filename, nil
}
