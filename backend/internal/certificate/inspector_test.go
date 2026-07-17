package certificate

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectorReadsCaddyCertificateFiles(t *testing.T) {
	dataDirectory := t.TempDir()
	certificateDirectory := filepath.Join(dataDirectory, "certificates", "acme.example", "wildcard_.example.com")
	if err := os.MkdirAll(certificateDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 17, 10, 0, 0, 0, time.UTC)
	certificateFile := filepath.Join(certificateDirectory, "wildcard_.example.com.crt")
	privateKeyFile := filepath.Join(certificateDirectory, "wildcard_.example.com.key")
	metadataFile := filepath.Join(certificateDirectory, "wildcard_.example.com.json")
	writeTestCertificate(t, certificateFile, now.Add(-60*24*time.Hour), now.Add(30*24*time.Hour))
	if err := os.WriteFile(privateKeyFile, []byte("test key path"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	inspector := NewInspector(dataDirectory)
	inspector.now = func() time.Time { return now }
	snapshot, err := inspector.Inspect(0)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if len(snapshot.Certificates) != 1 {
		t.Fatalf("certificates = %#v, want one", snapshot.Certificates)
	}
	status := snapshot.Certificates[0]
	if status.State != "renewal_due" {
		t.Fatalf("state = %q, want renewal_due", status.State)
	}
	if len(status.Subjects) != 2 || status.Subjects[0] != "*.example.com" || status.Subjects[1] != "example.com" {
		t.Fatalf("subjects = %#v", status.Subjects)
	}
	if status.Issuer != "Test ACME CA" || status.CertificateFile != certificateFile || status.PrivateKeyFile != privateKeyFile || status.MetadataFile != metadataFile {
		t.Fatalf("status = %#v", status)
	}
	if status.NotAfter != now.Add(30*24*time.Hour) || status.RemainingSeconds != int64((30*24*time.Hour).Seconds()) {
		t.Fatalf("validity = %#v", status)
	}
}

func TestInspectorReturnsEmptySnapshotBeforeFirstIssuance(t *testing.T) {
	inspector := NewInspector(t.TempDir())
	snapshot, err := inspector.Inspect(0)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if len(snapshot.Certificates) != 0 || snapshot.Certificates == nil {
		t.Fatalf("certificates = %#v, want non-nil empty slice", snapshot.Certificates)
	}
}

func writeTestCertificate(t *testing.T, path string, notBefore, notAfter time.Time) {
	t.Helper()
	issuerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test ACME CA"},
		NotBefore:             notBefore.Add(-time.Hour),
		NotAfter:              notAfter.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "*.example.com"},
		DNSNames:     []string{"example.com", "*.example.com"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	contents := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
