package config

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/aidockerfarm/gateway/internal/model"
	"github.com/aidockerfarm/gateway/internal/routes"
)

const (
	ConfigurationBundleFormat   = "caddyproxy-config"
	ConfigurationBundleVersion  = 1
	bundleManifestName          = "manifest.json"
	bundleRoutesName            = "routes.json"
	bundleSettingsName          = "settings.json"
	bundleCertificatePolicyName = "certificate-policy.json"
	maxBundleEntryBytes         = 1 << 20
	maxBundleTotalBytes         = 4 << 20
)

var configurationBundleFiles = []string{
	bundleManifestName,
	bundleRoutesName,
	bundleSettingsName,
	bundleCertificatePolicyName,
}

type ConfigurationBundleManifest struct {
	Format                      string    `json:"format"`
	Version                     int       `json:"version"`
	ExportedAt                  time.Time `json:"exportedAt"`
	Files                       []string  `json:"files"`
	CertificateMaterialIncluded bool      `json:"certificateMaterialIncluded"`
	SecretsIncluded             bool      `json:"secretsIncluded"`
}

type ConfigurationBundle struct {
	Manifest          ConfigurationBundleManifest
	Routes            routes.ResourceSet
	Settings          Settings
	CertificatePolicy model.CertificateConfig
}

func ExportConfigurationBundle(routeResources routes.ResourceSet, settings Settings, certificatePolicy model.CertificateConfig, exportedAt time.Time) ([]byte, error) {
	settings.Auth.AdminToken = ""
	settings.Auth.AdminTokens = nil
	settings.Auth.AdditionalHeaderName = ""
	settings.Auth.AdditionalHeaderValue = ""
	certificatePolicy.DNSChallenge.Azure.ClientSecret = ""
	manifest := ConfigurationBundleManifest{
		Format:                      ConfigurationBundleFormat,
		Version:                     ConfigurationBundleVersion,
		ExportedAt:                  exportedAt.UTC(),
		Files:                       append([]string(nil), configurationBundleFiles...),
		CertificateMaterialIncluded: false,
		SecretsIncluded:             false,
	}

	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, entry := range []struct {
		name  string
		value any
	}{
		{name: bundleManifestName, value: manifest},
		{name: bundleRoutesName, value: routeResources},
		{name: bundleSettingsName, value: settings},
		{name: bundleCertificatePolicyName, value: certificatePolicy},
	} {
		if err := writeConfigurationBundleEntry(archive, entry.name, entry.value, exportedAt); err != nil {
			_ = archive.Close()
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("close configuration archive: %w", err)
	}
	return output.Bytes(), nil
}

func ParseConfigurationBundle(data []byte) (ConfigurationBundle, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ConfigurationBundle{}, fmt.Errorf("open configuration archive: %w", err)
	}
	if len(reader.File) != len(configurationBundleFiles) {
		return ConfigurationBundle{}, fmt.Errorf("configuration archive must contain exactly %d files", len(configurationBundleFiles))
	}
	entries := make(map[string][]byte, len(reader.File))
	var total uint64
	for _, file := range reader.File {
		if !slices.Contains(configurationBundleFiles, file.Name) {
			return ConfigurationBundle{}, fmt.Errorf("configuration archive contains unsupported file %q", file.Name)
		}
		if _, exists := entries[file.Name]; exists {
			return ConfigurationBundle{}, fmt.Errorf("configuration archive contains duplicate file %q", file.Name)
		}
		if file.FileInfo().IsDir() || file.Mode()&os.ModeSymlink != 0 {
			return ConfigurationBundle{}, fmt.Errorf("configuration archive entry %q must be a regular file", file.Name)
		}
		if file.UncompressedSize64 > maxBundleEntryBytes || total+file.UncompressedSize64 > maxBundleTotalBytes {
			return ConfigurationBundle{}, fmt.Errorf("configuration archive exceeds the uncompressed size limit")
		}
		entry, err := file.Open()
		if err != nil {
			return ConfigurationBundle{}, fmt.Errorf("open configuration archive entry %q: %w", file.Name, err)
		}
		content, readErr := io.ReadAll(io.LimitReader(entry, maxBundleEntryBytes+1))
		closeErr := entry.Close()
		if readErr != nil {
			return ConfigurationBundle{}, fmt.Errorf("read configuration archive entry %q: %w", file.Name, readErr)
		}
		if closeErr != nil {
			return ConfigurationBundle{}, fmt.Errorf("close configuration archive entry %q: %w", file.Name, closeErr)
		}
		if len(content) > maxBundleEntryBytes {
			return ConfigurationBundle{}, fmt.Errorf("configuration archive entry %q exceeds the size limit", file.Name)
		}
		total += uint64(len(content))
		entries[file.Name] = content
	}

	var bundle ConfigurationBundle
	if err := decodeConfigurationBundleJSON(entries[bundleManifestName], &bundle.Manifest); err != nil {
		return ConfigurationBundle{}, fmt.Errorf("parse %s: %w", bundleManifestName, err)
	}
	if bundle.Manifest.Format != ConfigurationBundleFormat || bundle.Manifest.Version != ConfigurationBundleVersion {
		return ConfigurationBundle{}, fmt.Errorf("unsupported configuration bundle format %q version %d", bundle.Manifest.Format, bundle.Manifest.Version)
	}
	if !slices.Equal(bundle.Manifest.Files, configurationBundleFiles) || bundle.Manifest.CertificateMaterialIncluded || bundle.Manifest.SecretsIncluded {
		return ConfigurationBundle{}, fmt.Errorf("configuration bundle manifest does not match the supported safe file set")
	}
	if err := decodeConfigurationBundleJSON(entries[bundleRoutesName], &bundle.Routes); err != nil {
		return ConfigurationBundle{}, fmt.Errorf("parse %s: %w", bundleRoutesName, err)
	}
	if err := decodeConfigurationBundleJSON(entries[bundleSettingsName], &bundle.Settings); err != nil {
		return ConfigurationBundle{}, fmt.Errorf("parse %s: %w", bundleSettingsName, err)
	}
	if err := decodeConfigurationBundleJSON(entries[bundleCertificatePolicyName], &bundle.CertificatePolicy); err != nil {
		return ConfigurationBundle{}, fmt.Errorf("parse %s: %w", bundleCertificatePolicyName, err)
	}
	return bundle, nil
}

func writeConfigurationBundleEntry(archive *zip.Writer, name string, value any, modifiedAt time.Time) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(modifiedAt.UTC())
	entry, err := archive.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create configuration archive entry %q: %w", name, err)
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode configuration archive entry %q: %w", name, err)
	}
	return nil
}

func decodeConfigurationBundleJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("contains trailing JSON data")
	}
	return nil
}
