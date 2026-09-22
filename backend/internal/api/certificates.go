package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aidockerfarm/gateway/internal/certificate"
)

func (s *Server) withCertificateConfig(ctx context.Context, check func([]byte) error) error {
	if s.runtime != nil && !s.runtime.Ready() {
		return fmt.Errorf("Caddy runtime is not ready")
	}
	reader, ok := s.reconciler.(interface {
		WithRuntimeConfig(context.Context, func([]byte) error) error
	})
	if !ok {
		return fmt.Errorf("active Caddy configuration is unavailable")
	}
	return reader.WithRuntimeConfig(ctx, check)
}

func (s *Server) handleCertificateArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.rejectWhileConfigurationImportPending(w) {
		return
	}
	var request struct {
		ID          string `json:"id"`
		Fingerprint string `json:"fingerprintSha256"`
		Confirm     bool   `json:"confirm"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || !request.Confirm || len(request.ID) != 64 || len(request.Fingerprint) != 64 {
		writeError(w, http.StatusBadRequest, "certificate ID, fingerprint and explicit confirmation are required")
		return
	}
	archiver, ok := s.certificateInspector.(interface {
		Archive(context.Context, string, string, []byte) (certificate.ArchiveResult, error)
	})
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "certificate archive is unavailable")
		return
	}
	var result certificate.ArchiveResult
	err := s.withCertificateConfig(r.Context(), func(data []byte) error {
		var err error
		result, err = archiver.Archive(r.Context(), request.ID, request.Fingerprint, data)
		return err
	})
	if err != nil {
		s.audit("certificate.archive.rejected", map[string]any{"id": request.ID})
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.audit("certificate.archive", map[string]any{"id": result.ID, "directory": result.Directory})
	writeJSON(w, http.StatusOK, result)
}
