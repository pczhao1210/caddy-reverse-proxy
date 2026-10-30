package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func graphFixture() (*debug.BuildInfo, []sourcePackage) {
	info := &debug.BuildInfo{Path: "caddy", Main: debug.Module{Path: "caddy", Version: "(devel)"}, Deps: []*debug.Module{
		{Path: "golang.org/x/crypto", Version: "v0.58.0"},
		{Path: "github.com/go-jose/go-jose/v4", Version: "v4.1.5", Replace: &debug.Module{Path: "github.com/go-jose/go-jose/v4", Version: "v4.1.6"}},
		{Path: "go.step.sm/crypto", Version: "v0.91.1", Replace: &debug.Module{Path: "/opt/caddy-deps/crypto"}},
	}}
	packages := []sourcePackage{
		{ImportPath: "caddy", Module: &module{Path: "caddy", Main: true}},
		{ImportPath: "golang.org/x/crypto/ssh", Module: &module{Path: "golang.org/x/crypto", Version: "v0.58.0"}},
		{ImportPath: "github.com/go-jose/go-jose/v4", Module: &module{Path: "github.com/go-jose/go-jose/v4", Version: "v4.1.5", Replace: &module{Path: "github.com/go-jose/go-jose/v4", Version: "v4.1.6"}}},
		{ImportPath: "go.step.sm/crypto/jose", Module: &module{Path: "go.step.sm/crypto", Version: "v0.91.1", Replace: &module{Path: "/opt/caddy-deps/crypto"}}},
	}
	return info, packages
}

func TestVerifyGraph(t *testing.T) {
	for _, mode := range []string{"valid", "openpgp", "openpgp-child", "source-version", "extra-module", "missing-module", "missing-root", "source-error", "deps-error", "jose-v3", "old-jose-v4"} {
		t.Run(mode, func(t *testing.T) {
			info, packages := graphFixture()
			switch mode {
			case "openpgp":
				packages[1].ImportPath = "golang.org/x/crypto/openpgp"
			case "openpgp-child":
				packages[1].ImportPath = "golang.org/x/crypto/openpgp/armor"
			case "source-version":
				packages[1].Module.Version = "v0.57.0"
			case "extra-module":
				packages = append(packages, sourcePackage{ImportPath: "other", Module: &module{Path: "other", Version: "v1.0.0"}})
			case "missing-module":
				packages = packages[:len(packages)-1]
			case "missing-root":
				packages = packages[1:]
			case "source-error":
				packages[0].Error = &struct{ Err string }{"broken graph"}
			case "deps-error":
				packages[0].DepsErrors = []struct{ Err string }{{"missing dependency"}}
			case "jose-v3":
				info.Deps = append(info.Deps, &debug.Module{Path: "github.com/go-jose/go-jose/v3", Version: "v3.0.5"})
			case "old-jose-v4":
				info.Deps[1].Replace.Version = "v4.1.4"
			}
			version, count, err := verifyGraph(info, packages)
			if mode == "valid" {
				if err != nil || version != "v0.58.0" || count != 3 {
					t.Fatalf("version=%q count=%d err=%v", version, count, err)
				}
			} else if err == nil {
				t.Fatal("invalid source/binary proof was accepted")
			}
		})
	}
}

func TestWriteEvidenceIsNarrowlyScoped(t *testing.T) {
	directory := t.TempDir()
	result := evidence{Version: 1, Patches: map[string]string{"fixture.patch": strings.Repeat("a", 64)}, Sources: []sourceModule{
		{"go.step.sm/crypto", "v0.91.1", "h1:fixture"}, {"github.com/smallstep/certificates", "v0.30.2", "h1:fixture"},
	}, Artifacts: []artifact{
		{Path: "/usr/bin/caddy", SHA256: strings.Repeat("b", 64), GOOS: "linux", GOARCH: "amd64", CryptoVersion: "v0.58.0"},
		{Path: "/usr/local/bin/platform", SHA256: strings.Repeat("c", 64), GOOS: "linux", GOARCH: "amd64", CryptoVersion: "v0.58.0"},
	}}
	if err := writeEvidence(directory, result); err != nil {
		t.Fatal(err)
	}
	proof, err := os.ReadFile(filepath.Join(directory, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "openpgp.vex.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		ID         string `json:"@id"`
		Statements []struct {
			Vulnerability struct{ Name string } `json:"vulnerability"`
			Status        string
			Justification string
			Products      []struct {
				ID string `json:"@id"`
			}
		}
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.ID != "urn:sha256:"+digest(proof) || len(document.Statements) != 1 {
		t.Fatalf("VEX is not bound to the evidence: %s", data)
	}
	var bom struct {
		Components []struct{ Name, Version, PURL string }
	}
	data, err = os.ReadFile(filepath.Join(directory, "patched-dependencies.cdx.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &bom); err != nil {
		t.Fatal(err)
	}
	if len(bom.Components) != 2 || bom.Components[0].PURL != "pkg:golang/go.step.sm/crypto@v0.91.1" ||
		bom.Components[1].PURL != "pkg:golang/github.com/smallstep/certificates@v0.30.2" {
		t.Fatalf("patched source versions are not covered by the SBOM: %s", data)
	}
	statement := document.Statements[0]
	if statement.Vulnerability.Name != "GO-2026-5932" || statement.Status != "not_affected" || statement.Justification != "vulnerable_code_not_present" ||
		len(statement.Products) != 1 || statement.Products[0].ID != "pkg:golang/golang.org/x/crypto@v0.58.0" {
		t.Fatalf("unexpected VEX scope: %s", data)
	}
	result.Artifacts[1].GOARCH = "arm64"
	if err := writeEvidence(directory, result); err == nil {
		t.Fatal("mixed architecture evidence was accepted")
	}
}
