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

type ElevenLabsTTS struct {
	APIKey   string
	VoiceID  string
	AudioDir string
}

func (p *ElevenLabsTTS) Generate(ctx context.Context, text string) (string, error) {
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
		"text":     text,
		"model_id": "eleven_multilingual_v2",
	})

	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", p.VoiceID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("xi-api-key", p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("elevenlabs tts failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("elevenlabs tts error (status %d): %s", resp.StatusCode, string(b))
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
