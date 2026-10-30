package main

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

type module struct {
	Path    string
	Version string
	Main    bool
	Replace *module
}

type sourcePackage struct {
	ImportPath string
	Module     *module
	Error      *struct{ Err string }
	DepsErrors []struct{ Err string }
}

type artifact struct {
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	GOOS          string `json:"goos"`
	GOARCH        string `json:"goarch"`
	GoVersion     string `json:"goVersion"`
	CryptoVersion string `json:"cryptoVersion"`
	PackageCount  int    `json:"packageCount"`
	ModuleCount   int    `json:"moduleCount"`
	SourceSHA256  string `json:"sourceSHA256"`
}

type evidence struct {
	Version   int               `json:"version"`
	Artifacts []artifact        `json:"artifacts"`
	Patches   map[string]string `json:"patches"`
	Sources   []sourceModule    `json:"sources"`
}

type sourceModule struct {
	Path    string
	Version string
	Sum     string
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func identity(path, version string) string {
	if version == "(devel)" {
		version = ""
	}
	return path + "@" + version
}

func verifyGraph(info *debug.BuildInfo, packages []sourcePackage) (string, int, error) {
	expected := make(map[string]string)
	for _, dependency := range info.Deps {
		selected := dependency
		if selected.Replace != nil {
			selected = selected.Replace
		}
		expected[dependency.Path] = identity(selected.Path, selected.Version)
		if dependency.Path == "github.com/go-jose/go-jose/v3" {
			return "", 0, fmt.Errorf("binary still links JOSE v3")
		}
	}
	if strings.Contains(info.Path, "caddy") {
		if expected["github.com/go-jose/go-jose/v4"] != identity("github.com/go-jose/go-jose/v4", "v4.1.6") {
			return "", 0, fmt.Errorf("Caddy does not use JOSE v4.1.6")
		}
	}
	actual := make(map[string]string)
	foundRoot := false
	for _, pkg := range packages {
		if pkg.Error != nil || len(pkg.DepsErrors) != 0 {
			return "", 0, fmt.Errorf("incomplete source graph at %s", pkg.ImportPath)
		}
		if pkg.ImportPath == "golang.org/x/crypto/openpgp" || strings.HasPrefix(pkg.ImportPath, "golang.org/x/crypto/openpgp/") {
			return "", 0, fmt.Errorf("source graph imports vulnerable package %s", pkg.ImportPath)
		}
		if pkg.ImportPath == info.Path && pkg.Module != nil && pkg.Module.Main && identity(pkg.Module.Path, pkg.Module.Version) == identity(info.Main.Path, info.Main.Version) {
			foundRoot = true
		}
		if pkg.Module == nil || pkg.Module.Main {
			continue
		}
		selected := pkg.Module
		if selected.Replace != nil {
			selected = selected.Replace
		}
		actual[pkg.Module.Path] = identity(selected.Path, selected.Version)
	}
	if !foundRoot || len(actual) == 0 || len(actual) != len(expected) {
		return "", 0, fmt.Errorf("source/binary module count mismatch: %d/%d", len(actual), len(expected))
	}
	for path, selected := range expected {
		if actual[path] != selected {
			return "", 0, fmt.Errorf("source/binary module mismatch for %s: %q/%q", path, actual[path], selected)
		}
	}
	for _, dependency := range info.Deps {
		if dependency.Path == "golang.org/x/crypto" && dependency.Replace == nil && dependency.Version != "" {
			return dependency.Version, len(actual), nil
		}
		if dependency.Path == "golang.org/x/crypto" && dependency.Replace != nil && dependency.Replace.Path == dependency.Path && dependency.Replace.Version != "" {
			return dependency.Replace.Version, len(actual), nil
		}
	}
	return "", 0, fmt.Errorf("versioned x/crypto build metadata is missing")
}

func inspect(binary, graph, path string) (artifact, error) {
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return artifact{}, fmt.Errorf("read %s build metadata: %w", path, err)
	}
	data, err := os.ReadFile(graph)
	if err != nil {
		return artifact{}, err
	}
	var packages []sourcePackage
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	for {
		var pkg sourcePackage
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			return artifact{}, fmt.Errorf("decode %s graph: %w", path, err)
		}
		packages = append(packages, pkg)
	}
	cryptoVersion, moduleCount, err := verifyGraph(info, packages)
	if err != nil {
		return artifact{}, fmt.Errorf("%s: %w", path, err)
	}
	binaryData, err := os.ReadFile(binary)
	if err != nil {
		return artifact{}, err
	}
	result := artifact{Path: path, SHA256: digest(binaryData), GoVersion: info.GoVersion,
		CryptoVersion: cryptoVersion, PackageCount: len(packages), ModuleCount: moduleCount}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "GOOS":
			result.GOOS = setting.Value
		case "GOARCH":
			result.GOARCH = setting.Value
		}
	}
	if result.GOOS != "linux" || (result.GOARCH != "amd64" && result.GOARCH != "arm64") {
		return artifact{}, fmt.Errorf("%s: unsupported or missing build target %s/%s", path, result.GOOS, result.GOARCH)
	}
	canonical := make([]string, 0, len(packages))
	for _, pkg := range packages {
		canonical = append(canonical, pkg.ImportPath)
	}
	slices.Sort(canonical)
	result.SourceSHA256 = digest([]byte(strings.Join(canonical, "\n")))
	return result, nil
}

func writeEvidence(out string, result evidence) error {
	if len(result.Artifacts) != 2 || result.Artifacts[0].GOARCH != result.Artifacts[1].GOARCH || len(result.Sources) != 2 {
		return fmt.Errorf("both matching-architecture artifacts are required")
	}
	proof, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	proof = append(proof, '\n')
	type product struct {
		ID string `json:"@id"`
	}
	products := []product{}
	for _, item := range result.Artifacts {
		value := product{ID: "pkg:golang/golang.org/x/crypto@" + item.CryptoVersion}
		if !slices.Contains(products, value) {
			products = append(products, value)
		}
	}
	document := map[string]any{
		"@context": "https://openvex.dev/ns/v0.2.0", "@id": "urn:sha256:" + digest(proof),
		"author": "pczhao1210/caddy-reverse-proxy", "timestamp": time.Now().UTC().Format(time.RFC3339), "version": 1,
		"statements": []any{map[string]any{
			"vulnerability": map[string]string{"name": "GO-2026-5932"}, "products": products,
			"status": "not_affected", "justification": "vulnerable_code_not_present",
			"impact_statement": "No openpgp packages are imported by either source graph. The scan-image wrapper must verify the attached evidence and both binary hashes before applying this statement; it is not a module-wide exemption.",
		}},
	}
	vex, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "evidence.json"), proof, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "openpgp.vex.json"), append(vex, '\n'), 0o644); err != nil {
		return err
	}
	components := make([]map[string]any, 0, len(result.Sources))
	for _, source := range result.Sources {
		components = append(components, map[string]any{
			"type": "library", "name": source.Path, "version": source.Version,
			"purl":       "pkg:golang/" + source.Path + "@" + source.Version,
			"properties": []map[string]string{{"name": "gateway:upstream:go-checksum", "value": source.Sum}},
		})
	}
	bom, err := json.MarshalIndent(map[string]any{
		"bomFormat": "CycloneDX", "specVersion": "1.6", "version": 1, "components": components,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "patched-dependencies.cdx.json"), append(bom, '\n'), 0o644)
}

func run() error {
	caddy := flag.String("caddy", "", "Caddy executable")
	platform := flag.String("platform", "", "Control-plane executable")
	caddyGraph := flag.String("caddy-graph", "", "Exact Caddy go list -deps -json output")
	platformGraph := flag.String("platform-graph", "", "Exact control-plane go list -deps -json output")
	patches := flag.String("patches", "", "Audited dependency patch directory")
	sources := flag.String("sources", "", "Verified upstream go mod download metadata")
	out := flag.String("out", "", "Output directory")
	flag.Parse()
	if *caddy == "" || *platform == "" || *caddyGraph == "" || *platformGraph == "" || *patches == "" || *sources == "" || *out == "" || flag.NArg() != 0 {
		return fmt.Errorf("all executable, graph, patch and output paths are required")
	}
	result := evidence{Version: 1, Patches: make(map[string]string)}
	data, err := os.ReadFile(*sources)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	for _, expected := range []sourceModule{
		{"go.step.sm/crypto", "v0.91.1", "h1:ZMMdj/A2rQ+Zr8hyCo4BHJd++c5T8VQIXJ8Sljg57p0="},
		{"github.com/smallstep/certificates", "v0.30.2", "h1:1G3xBi8sJ740iA1mMPW2Svv7EIZKJ4Zf/iQtA5QlN0Y="},
	} {
		var actual sourceModule
		if err := decoder.Decode(&actual); err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("unexpected patched source provenance for %s", expected.Path)
		}
		result.Sources = append(result.Sources, actual)
	}
	var extra sourceModule
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected additional source metadata")
	}
	for _, input := range []struct{ binary, graph, path string }{
		{*caddy, *caddyGraph, "/usr/bin/caddy"}, {*platform, *platformGraph, "/usr/local/bin/platform"},
	} {
		item, err := inspect(input.binary, input.graph, input.path)
		if err != nil {
			return err
		}
		result.Artifacts = append(result.Artifacts, item)
	}
	for _, name := range []string{"step-crypto-jose-v4.patch", "step-certificates-jose-v4.patch"} {
		data, err := os.ReadFile(filepath.Join(*patches, name))
		if err != nil {
			return err
		}
		result.Patches[name] = digest(data)
	}
	return writeEvidence(*out, result)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
