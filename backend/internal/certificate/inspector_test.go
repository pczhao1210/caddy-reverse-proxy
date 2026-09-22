package certificate

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidockerfarm/gateway/internal/logs"
	"github.com/caddyserver/certmagic"
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

func TestClassifyCertificatePolicy(t *testing.T) {
	snapshot := Snapshot{Certificates: []Status{
		{Subjects: []string{"*.example.com"}},
		{Subjects: []string{"app.example.com"}, State: "renewal_due"},
		{Subjects: []string{"example.com"}},
		{Subjects: []string{"nested.app.example.com"}},
		{Subjects: []string{"app.example.com", "required.net"}},
	}}
	Classify(&snapshot, Policy{ManagedSubjects: []string{"*.example.com", "required.net"}})
	want := []string{"managed", "covered", "unreferenced", "unreferenced", "managed"}
	for index, status := range snapshot.Certificates {
		if status.Usage != want[index] || status.CanArchive {
			t.Fatalf("classification %d: %+v", index, status)
		}
	}
	if snapshot.Certificates[1].State != "renewal_due" || snapshot.Certificates[1].CoveredBy[0] != "*.example.com" {
		t.Fatal("classification must retain validity separately from usage")
	}
	if !Covers("*.EXAMPLE.com.", "App.example.com") || Covers("*.example.com", "example.com") || Covers("*.example.com", "nested.app.example.com") {
		t.Fatal("wildcard coverage must match exactly one label")
	}
}

func TestParseActiveCertificatePolicy(t *testing.T) {
	config := `{"storage":{"module":"file_system","root":"/data/caddy"},"apps":{"tls":{"certificates":{"automate":["*.example.com"]}},"http":{"servers":{"https":{"listen":[":443"],"tls_connection_policies":[{}],"routes":[{"match":[{"host":["app.example.com","example.com","deep.app.example.com"]}]}]}}}}}`
	policy, err := ParsePolicy([]byte(config), "/data/caddy")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(policy.ManagedSubjects) != "[*.example.com deep.app.example.com example.com]" || len(policy.Endpoints) != 3 {
		t.Fatalf("policy=%+v", policy)
	}
	if _, err := ParsePolicy([]byte(config), "/other"); err == nil {
		t.Fatal("mismatched storage accepted")
	}
	for _, unsafe := range []string{
		`{"certificates":{"load_pem":[]}}`,
		`{"automation":{"policies":[{"on_demand":true}]}}`,
		`{"automation":{"policies":[{"get_certificate":{}}]}}`,
	} {
		data := `{"storage":{"module":"file_system","root":"/data/caddy"},"apps":{"tls":` + unsafe + `}}`
		if _, err := ParsePolicy([]byte(data), "/data/caddy"); err == nil {
			t.Fatalf("unsafe policy accepted: %s", unsafe)
		}
	}
}

func TestArchiveCertificateDirectory(t *testing.T) {
	for _, mode := range []string{"history", "managed", "stale", "extra-file", "symlink", "locked", "missing-key", "path-id", "directory-symlink"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "certificates", "test-ca", "wildcard.example.com", "wildcard.example.com.crt")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			writeTestCertificate(t, path, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
			for _, extension := range []string{".key", ".json"} {
				if err := os.WriteFile(strings.TrimSuffix(path, ".crt")+extension, []byte("fixture"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			inspector := NewInspector(root)
			snapshot, err := inspector.Inspect(0)
			if err != nil {
				t.Fatal(err)
			}
			certificate := snapshot.Certificates[0]
			subjects := `[]`
			switch mode {
			case "managed":
				subjects = `["app.example.com"]`
			case "stale":
				certificate.FingerprintSHA256 = "changed"
			case "path-id":
				certificate.ID = "../../outside"
			case "missing-key":
				if err := os.Remove(strings.TrimSuffix(path, ".crt") + ".key"); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				original := filepath.Dir(path)
				if err := os.Rename(original, original+"-real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(original+"-real", original); err != nil {
					t.Fatal(err)
				}
			case "extra-file":
				if err := os.WriteFile(filepath.Join(filepath.Dir(path), "other.crt"), []byte("invalid"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				key := strings.TrimSuffix(path, ".crt") + ".key"
				if err := os.Remove(key); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path, key); err != nil {
					t.Fatal(err)
				}
			}
			config := fmt.Sprintf(`{"storage":{"module":"file_system","root":%q},"apps":{"tls":{"certificates":{"automate":%s}}}}`, root, subjects)
			ctx := context.Background()
			if mode == "locked" {
				storage := &certmagic.FileStorage{Path: root}
				if err := storage.Lock(ctx, "issue_cert_*.example.com"); err != nil {
					t.Fatal(err)
				}
				defer storage.Unlock(ctx, "issue_cert_*.example.com")
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
			}
			result, err := inspector.Archive(ctx, certificate.ID, certificate.FingerprintSHA256, []byte(config))
			if mode != "history" {
				if err == nil {
					t.Fatal("unsafe archive accepted")
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatal("original certificate was removed", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("original certificate directory remains")
			}
			if _, err := os.Stat(filepath.Join(result.Directory, filepath.Base(path))); err != nil {
				t.Fatal(err)
			}
			manifest, err := os.ReadFile(filepath.Join(filepath.Dir(result.Directory), "manifest.json"))
			if err != nil || !strings.Contains(string(manifest), "originalDirectory") {
				t.Fatalf("missing restore manifest: %s %v", manifest, err)
			}
			snapshot, err = inspector.Inspect(0)
			if err != nil || len(snapshot.Certificates) != 0 {
				t.Fatalf("archive still appears in inventory: %+v %v", snapshot, err)
			}
		})
	}
}

func TestArchiveRefusesServedCertificate(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	leaf := server.Certificate()
	fingerprint := sha256.Sum256(leaf.Raw)
	target := Status{Subjects: leaf.DNSNames, FingerprintSHA256: hex.EncodeToString(fingerprint[:])}
	policy := Policy{Endpoints: []Endpoint{{Address: server.Listener.Addr().String(), Host: leaf.DNSNames[0]}}}
	if err := verifyNotServing(context.Background(), target, policy); err == nil || !strings.Contains(err.Error(), "still served") {
		t.Fatalf("served certificate accepted: %v", err)
	}
	target.FingerprintSHA256 = "another certificate"
	if err := verifyNotServing(context.Background(), target, policy); err != nil {
		t.Fatal(err)
	}
}

func TestCertificateRecentEventsExcludeSecrets(t *testing.T) {
	store := logs.NewStore(100)
	writer := store.Writer("caddy/stderr", "error")
	for _, line := range []string{
		`{"ts":1700000000,"level":"info","logger":"tls.renew","msg":"certificate renewed successfully","identifier":"app.example.com"}`,
		`{"ts":1700000001,"level":"error","logger":"tls.renew","msg":"will retry","retrying_in":60,"error":"[app.example.com] Renew: DNS propagation failed client_secret=secret-value","client_secret":"secret-value"}`,
		`{"ts":1700000002,"level":"error","logger":"http","msg":"not a certificate event"}`,
	} {
		if _, err := writer.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	events := RecentEvents(store.ReadLast(100))
	if len(events) == 0 || len(events[0].Subjects) != 1 || events[0].Subjects[0] != "app.example.com" {
		t.Fatalf("retry identifier missing: %+v", events)
	}
	if len(events) != 2 || events[0].Outcome != "retry" || events[0].Operation != "renew" || events[0].ErrorCode != "dns" || events[0].RetryAt == nil || events[0].RetryAt.Unix() != 1700000061 || events[1].Outcome != "success" {
		t.Fatalf("events=%+v", events)
	}
	encoded, err := json.Marshal(events)
	if err != nil || strings.Contains(string(encoded), "secret") {
		t.Fatalf("unsafe events: %s %v", encoded, err)
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
