package tts

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
)

// Generator defines the interface for Text-To-Speech providers
type Generator interface {
	// Generate converts text to an audio file and returns the filename.
	Generate(ctx context.Context, text string) (string, error)
}

// sha256Short returns the first 16 hex characters of the SHA256 hash of s.
func sha256Short(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}

// fileExists returns true if the file at path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
