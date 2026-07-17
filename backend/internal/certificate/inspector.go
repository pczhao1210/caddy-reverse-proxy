package certificate

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

const defaultRenewalWindowRatio = 1.0 / 3.0

type Status struct {
	ID                 string    `json:"id"`
	State              string    `json:"state"`
	Subjects           []string  `json:"subjects"`
	Issuer             string    `json:"issuer"`
	SerialNumber       string    `json:"serialNumber"`
	NotBefore          time.Time `json:"notBefore"`
	NotAfter           time.Time `json:"notAfter"`
	RemainingSeconds   int64     `json:"remainingSeconds"`
	RenewalWindowStart time.Time `json:"renewalWindowStart"`
	CertificateFile    string    `json:"certificateFile"`
	PrivateKeyFile     string    `json:"privateKeyFile,omitempty"`
	MetadataFile       string    `json:"metadataFile,omitempty"`
	FingerprintSHA256  string    `json:"fingerprintSha256"`
}

type Snapshot struct {
	StorageDirectory string    `json:"storageDirectory"`
	ScannedAt        time.Time `json:"scannedAt"`
	Certificates     []Status  `json:"certificates"`
	Warnings         []string  `json:"warnings,omitempty"`
}

type Inspector struct {
	dataDirectory string
	now           func() time.Time
}

func NewInspector(dataDirectory string) *Inspector {
	return &Inspector{dataDirectory: filepath.Clean(dataDirectory), now: time.Now}
}

func (i *Inspector) Inspect(renewalWindowRatio float64) (Snapshot, error) {
	if renewalWindowRatio <= 0 || renewalWindowRatio >= 1 {
		renewalWindowRatio = defaultRenewalWindowRatio
	}
	now := i.now().UTC()
	root := filepath.Join(i.dataDirectory, "certificates")
	snapshot := Snapshot{
		StorageDirectory: i.dataDirectory,
		ScannedAt:        now,
		Certificates:     []Status{},
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return snapshot, nil
		}
		return snapshot, fmt.Errorf("inspect certificate storage: %w", err)
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".crt") {
			return nil
		}
		status, err := i.inspectFile(path, now, renewalWindowRatio)
		if err != nil {
			snapshot.Warnings = append(snapshot.Warnings, err.Error())
			return nil
		}
		snapshot.Certificates = append(snapshot.Certificates, status)
		return nil
	})
	if err != nil {
		return snapshot, fmt.Errorf("walk certificate storage: %w", err)
	}
	sort.Slice(snapshot.Certificates, func(left, right int) bool {
		if snapshot.Certificates[left].NotAfter.Equal(snapshot.Certificates[right].NotAfter) {
			return snapshot.Certificates[left].CertificateFile < snapshot.Certificates[right].CertificateFile
		}
		return snapshot.Certificates[left].NotAfter.Before(snapshot.Certificates[right].NotAfter)
	})
	return snapshot, nil
}

func (i *Inspector) inspectFile(path string, now time.Time, renewalWindowRatio float64) (Status, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Status{}, fmt.Errorf("read certificate %s: %w", path, err)
	}
	block, _ := pem.Decode(contents)
	if block == nil || block.Type != "CERTIFICATE" {
		return Status{}, fmt.Errorf("parse certificate %s: no PEM certificate found", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Status{}, fmt.Errorf("parse certificate %s: %w", path, err)
	}

	subjects := slices.Clone(cert.DNSNames)
	if len(subjects) == 0 && cert.Subject.CommonName != "" {
		subjects = []string{cert.Subject.CommonName}
	}
	slices.Sort(subjects)
	fingerprint := sha256.Sum256(cert.Raw)
	renewalWindowStart := cert.NotAfter.Add(-time.Duration(float64(cert.NotAfter.Sub(cert.NotBefore)) * renewalWindowRatio))
	state := "valid"
	switch {
	case now.Before(cert.NotBefore):
		state = "not_yet_valid"
	case !now.Before(cert.NotAfter):
		state = "expired"
	case !now.Before(renewalWindowStart):
		state = "renewal_due"
	}

	base := strings.TrimSuffix(path, filepath.Ext(path))
	return Status{
		ID:                 hex.EncodeToString(fingerprint[:]),
		State:              state,
		Subjects:           subjects,
		Issuer:             certificateIssuer(cert),
		SerialNumber:       cert.SerialNumber.Text(16),
		NotBefore:          cert.NotBefore.UTC(),
		NotAfter:           cert.NotAfter.UTC(),
		RemainingSeconds:   int64(cert.NotAfter.Sub(now).Seconds()),
		RenewalWindowStart: renewalWindowStart.UTC(),
		CertificateFile:    path,
		PrivateKeyFile:     existingFile(base + ".key"),
		MetadataFile:       existingFile(base + ".json"),
		FingerprintSHA256:  hex.EncodeToString(fingerprint[:]),
	}, nil
}

func certificateIssuer(cert *x509.Certificate) string {
	if cert.Issuer.CommonName != "" {
		return cert.Issuer.CommonName
	}
	return cert.Issuer.String()
}

func existingFile(path string) string {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return path
	}
	return ""
}
