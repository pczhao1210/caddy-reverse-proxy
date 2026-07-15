package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aidockerfarm/gateway/internal/model"
)

type CertificateStore struct {
	path string
}

func NewCertificateStore(path string) *CertificateStore {
	return &CertificateStore{path: path}
}

func (s *CertificateStore) Load() (model.CertificateConfig, bool, error) {
	if s == nil || s.path == "" {
		return model.CertificateConfig{}, false, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return model.CertificateConfig{}, false, nil
	}
	if err != nil {
		return model.CertificateConfig{}, false, fmt.Errorf("read certificate config: %w", err)
	}
	var certificate model.CertificateConfig
	if err := json.Unmarshal(data, &certificate); err != nil {
		return model.CertificateConfig{}, false, fmt.Errorf("parse certificate config: %w", err)
	}
	return certificate, true, nil
}

func (s *CertificateStore) Save(certificate model.CertificateConfig) error {
	if s == nil || s.path == "" {
		return fmt.Errorf("certificate config file is not configured")
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create certificate config directory: %w", err)
	}
	data, err := json.MarshalIndent(certificate, "", "  ")
	if err != nil {
		return fmt.Errorf("encode certificate config: %w", err)
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary certificate config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace certificate config: %w", err)
	}
	return nil
}
