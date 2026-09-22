package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/persistence"
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
	if err := persistence.WriteJSON(s.path, certificate, 0o700); err != nil {
		return fmt.Errorf("save certificate config: %w", err)
	}
	return nil
}
