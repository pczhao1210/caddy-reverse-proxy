package certificate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
)

type ArchiveResult struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
}

func (i *Inspector) Archive(ctx context.Context, id, fingerprint string, config []byte) (ArchiveResult, error) {
	policy, err := ParsePolicy(config, i.dataDirectory)
	if err != nil {
		return ArchiveResult{}, err
	}
	snapshot, err := i.Inspect(defaultRenewalWindowRatio)
	if err != nil {
		return ArchiveResult{}, err
	}
	Classify(&snapshot, policy)
	var target *Status
	for index := range snapshot.Certificates {
		if snapshot.Certificates[index].ID == id {
			target = &snapshot.Certificates[index]
		}
	}
	if target == nil || target.FingerprintSHA256 != fingerprint {
		return ArchiveResult{}, fmt.Errorf("certificate changed or no longer exists; refresh the inventory")
	}
	if !target.CanArchive {
		return ArchiveResult{}, fmt.Errorf("certificate is managed or its usage cannot be verified")
	}
	root, err := os.OpenRoot(i.dataDirectory)
	if err != nil {
		return ArchiveResult{}, err
	}
	defer root.Close()
	if err := root.Mkdir("locks", 0o700); err != nil && !os.IsExist(err) {
		return ArchiveResult{}, err
	}
	if err := ensurePlainPath(root, "locks"); err != nil {
		return ArchiveResult{}, err
	}
	storage := &certmagic.FileStorage{Path: i.dataDirectory}
	lockContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	lockNames := []string{"storage_clean"}
	for _, subject := range target.Subjects {
		lockNames = append(lockNames, "issue_cert_"+normalizeName(subject))
	}
	slices.Sort(lockNames)
	lockNames = slices.Compact(lockNames)
	for _, name := range lockNames {
		if err := storage.Lock(lockContext, name); err != nil {
			return ArchiveResult{}, fmt.Errorf("certificate storage is busy: %w", err)
		}
		defer storage.Unlock(context.WithoutCancel(ctx), name)
	}
	relative, err := filepath.Rel(i.dataDirectory, target.CertificateFile)
	if err != nil {
		return ArchiveResult{}, err
	}
	directory := filepath.Dir(relative)
	if !filepath.IsLocal(relative) || len(strings.Split(directory, string(filepath.Separator))) != 3 || !strings.HasPrefix(directory, "certificates"+string(filepath.Separator)) {
		return ArchiveResult{}, fmt.Errorf("only standard Caddy certificate directories can be archived")
	}
	contents, err := archiveContents(root, directory, filepath.Base(relative))
	if err != nil {
		return ArchiveResult{}, err
	}
	certificatePEM, err := root.ReadFile(relative)
	if err != nil {
		return ArchiveResult{}, err
	}
	block, _ := pem.Decode(certificatePEM)
	if block == nil {
		return ArchiveResult{}, fmt.Errorf("certificate changed during validation")
	}
	actualFingerprint := sha256.Sum256(block.Bytes)
	if hex.EncodeToString(actualFingerprint[:]) != fingerprint || sha256.Sum256(certificatePEM) != contents[filepath.Base(relative)].hash {
		return ArchiveResult{}, fmt.Errorf("certificate changed during validation")
	}
	if err := verifyNotServing(ctx, *target, policy); err != nil {
		return ArchiveResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ArchiveResult{}, err
	}
	archiveID := rand.Text()
	archiveDirectory := filepath.Join("certificate-archive", archiveID)
	destination := filepath.Join(archiveDirectory, "materials")
	if err := root.Mkdir("certificate-archive", 0o700); err != nil && !os.IsExist(err) {
		return ArchiveResult{}, err
	}
	if err := ensurePlainPath(root, "certificate-archive"); err != nil {
		return ArchiveResult{}, err
	}
	if err := root.Chmod("certificate-archive", 0o700); err != nil {
		return ArchiveResult{}, err
	}
	if err := root.Mkdir(archiveDirectory, 0o700); err != nil {
		return ArchiveResult{}, err
	}
	manifest, err := root.OpenFile(filepath.Join(archiveDirectory, "manifest.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ArchiveResult{}, err
	}
	encodeErr := json.NewEncoder(manifest).Encode(map[string]any{"originalDirectory": directory, "fingerprintSha256": fingerprint, "archivedAt": i.now().UTC()})
	syncErr := manifest.Sync()
	closeErr := manifest.Close()
	if encodeErr != nil || syncErr != nil || closeErr != nil {
		return ArchiveResult{}, fmt.Errorf("cannot persist archive manifest")
	}
	latest, err := archiveContents(root, directory, filepath.Base(relative))
	if err != nil || !sameContents(contents, latest) {
		return ArchiveResult{}, fmt.Errorf("certificate files changed during validation; retry after refresh")
	}
	if err := root.Rename(directory, destination); err != nil {
		return ArchiveResult{}, fmt.Errorf("archive certificate directory: %w", err)
	}
	archived, verifyErr := archiveContents(root, destination, filepath.Base(relative))
	if verifyErr != nil || !sameContents(contents, archived) {
		if _, err := root.Lstat(directory); !os.IsNotExist(err) {
			return ArchiveResult{}, fmt.Errorf("certificate changed while archiving; original path was recreated; manual recovery required from %s", destination)
		}
		if restoreErr := root.Rename(destination, directory); restoreErr != nil {
			return ArchiveResult{}, fmt.Errorf("certificate changed while archiving; manual recovery required from %s: %w", destination, restoreErr)
		}
		return ArchiveResult{}, fmt.Errorf("certificate changed while archiving; original directory restored")
	}
	return ArchiveResult{ID: id, Directory: filepath.Join(i.dataDirectory, destination)}, nil
}

func ensurePlainPath(root *os.Root, path string) error {
	current := ""
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links cannot be archived")
		}
	}
	return nil
}

type archivedFile struct {
	info os.FileInfo
	hash [32]byte
}

func archiveContents(root *os.Root, directory, certificateName string) (map[string]archivedFile, error) {
	if err := ensurePlainPath(root, directory); err != nil {
		return nil, err
	}
	folder, err := root.Open(directory)
	if err != nil {
		return nil, err
	}
	entries, err := folder.ReadDir(-1)
	_ = folder.Close()
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(certificateName, ".crt")
	if len(entries) != 3 {
		return nil, fmt.Errorf("certificate directory must contain exactly one certificate, key and metadata file")
	}
	contents := make(map[string]archivedFile, 3)
	for _, entry := range entries {
		if entry.Name() != base+".crt" && entry.Name() != base+".key" && entry.Name() != base+".json" {
			return nil, fmt.Errorf("certificate directory contains unexpected files")
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
			return nil, fmt.Errorf("certificate files must be regular files smaller than 1 MiB")
		}
		file, err := root.Open(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 1024*1024+1))
		opened, statErr := file.Stat()
		_ = file.Close()
		if readErr != nil || statErr != nil || !os.SameFile(info, opened) || len(data) > 1024*1024 {
			return nil, fmt.Errorf("certificate file changed while reading")
		}
		contents[entry.Name()] = archivedFile{info: info, hash: sha256.Sum256(data)}
	}
	return contents, nil
}

func sameContents(before, after map[string]archivedFile) bool {
	if len(before) != len(after) {
		return false
	}
	for name, original := range before {
		current, ok := after[name]
		if !ok || original.hash != current.hash || !os.SameFile(original.info, current.info) || !original.info.ModTime().Equal(current.info.ModTime()) {
			return false
		}
	}
	return true
}

func verifyNotServing(ctx context.Context, target Status, policy Policy) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	checked := make(map[Endpoint]bool)
	for _, endpoint := range policy.Endpoints {
		affected := false
		for _, subject := range target.Subjects {
			if Covers(subject, endpoint.Host) || Covers(endpoint.Host, subject) {
				affected = true
				if strings.Contains(endpoint.Host, "*") {
					return fmt.Errorf("wildcard routes require operator review before archiving")
				}
			}
		}
		if !affected || checked[endpoint] {
			continue
		}
		checked[endpoint] = true
		dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 3 * time.Second}, Config: &tls.Config{ServerName: endpoint.Host, InsecureSkipVerify: true}}
		connection, err := dialer.DialContext(ctx, "tcp", endpoint.Address)
		if err != nil {
			return fmt.Errorf("cannot verify certificate served for %s: %w", endpoint.Host, err)
		}
		state := connection.(*tls.Conn).ConnectionState()
		_ = connection.Close()
		if len(state.PeerCertificates) == 0 {
			return fmt.Errorf("no certificate served for %s", endpoint.Host)
		}
		served := state.PeerCertificates[0]
		fingerprint := sha256.Sum256(served.Raw)
		if hex.EncodeToString(fingerprint[:]) == target.FingerprintSHA256 {
			return fmt.Errorf("certificate is still served for %s", endpoint.Host)
		}
		now := time.Now()
		if served.VerifyHostname(endpoint.Host) != nil || now.Before(served.NotBefore) || !now.Before(served.NotAfter) {
			return fmt.Errorf("replacement certificate for %s is not valid", endpoint.Host)
		}
	}
	return nil
}
