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
	"strings"

	"golang.org/x/oauth2/google"
)

type GoogleTTS struct {
	CredentialsJSON []byte
	VoiceName       string
	AudioDir        string
}

func (p *GoogleTTS) Generate(ctx context.Context, text string) (string, error) {
	hash := sha256Short(text)
	filename := "tts_" + hash + ".mp3"
	outPath := filepath.Join(p.AudioDir, filename)

	if fileExists(outPath) {
		return filename, nil
	}

	if err := os.MkdirAll(p.AudioDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create audio directory: %w", err)
	}

	creds, err := google.CredentialsFromJSONWithTypeAndParams(
		ctx,
		p.CredentialsJSON,
		google.ServiceAccount,
		google.CredentialsParams{
			Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
		},
	)
	if err != nil {
		return "", fmt.Errorf("google tts invalid credentials: %w", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("google tts token error: %w", err)
	}

	langCode := "en-US"
	if parts := strings.Split(p.VoiceName, "-"); len(parts) >= 2 {
		langCode = parts[0] + "-" + parts[1]
	}

	reqBody, _ := json.Marshal(map[string]any{
		"input": map[string]string{
			"text": text,
		},
		"voice": map[string]string{
			"languageCode": langCode,
			"name":         p.VoiceName,
		},
		"audioConfig": map[string]string{
			"audioEncoding": "MP3",
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://texttospeech.googleapis.com/v1/text:synthesize", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-User-Project", creds.ProjectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("google tts failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("google tts error (status %d): %s", resp.StatusCode, string(b))
	}

	var resData struct {
		AudioContent []byte `json:"audioContent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		return "", fmt.Errorf("google tts bad response: %w", err)
	}

	if err := os.WriteFile(outPath, resData.AudioContent, 0644); err != nil {
		return "", err
	}

	return filename, nil
}
