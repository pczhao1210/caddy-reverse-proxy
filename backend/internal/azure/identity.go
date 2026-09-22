package azure

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadInstanceID(directory string) (string, error) {
	if directory == "" {
		return "", fmt.Errorf("state directory is required for Azure resource ownership")
	}
	path := filepath.Join(directory, "azure-instance-id")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return "", err
		}
		identifier := make([]byte, 16)
		if _, err := rand.Read(identifier); err != nil {
			return "", err
		}
		temporary, createErr := os.CreateTemp(directory, ".azure-instance-id-*")
		if createErr != nil {
			return "", createErr
		}
		defer os.Remove(temporary.Name())
		_, writeErr := temporary.WriteString(hex.EncodeToString(identifier) + "\n")
		syncErr := temporary.Sync()
		closeErr := temporary.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return "", err
		}
		if err := os.Link(temporary.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	identifier := strings.TrimSpace(string(data))
	decoded, err := hex.DecodeString(identifier)
	if err != nil || len(decoded) != 16 {
		return "", fmt.Errorf("invalid Azure instance identity in %s", path)
	}
	return identifier, nil
}
