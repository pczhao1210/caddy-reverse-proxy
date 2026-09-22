package certificate

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strings"
)

type Policy struct {
	ManagedSubjects []string
	Endpoints       []Endpoint
	verified        bool
}

type Endpoint struct {
	Address string
	Host    string
}

func ParsePolicy(data []byte, storageDirectory string) (Policy, error) {
	var config struct {
		Storage struct {
			Module string
			Root   string
		}
		Apps struct {
			TLS struct {
				Certificates map[string]json.RawMessage
				Automation   struct {
					Policies []struct {
						OnDemand bool            `json:"on_demand"`
						Managers json.RawMessage `json:"get_certificate"`
					}
				}
			}
			HTTP struct {
				Servers map[string]struct {
					Listen    []string
					TLS       []map[string]json.RawMessage `json:"tls_connection_policies"`
					Automatic struct {
						Disabled            bool `json:"disable"`
						DisableCertificates bool `json:"disable_certificates"`
						Skip                []string
						SkipCertificates    []string `json:"skip_certificates"`
					} `json:"automatic_https"`
					Routes []struct {
						Match  []struct{ Host []string }
						Handle []struct{ Handler string }
					}
				}
			}
		}
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return Policy{}, err
	}
	if config.Storage.Module != "file_system" || filepath.Clean(config.Storage.Root) != filepath.Clean(storageDirectory) {
		return Policy{}, fmt.Errorf("active certificate storage does not match the inspected directory")
	}
	policy := Policy{verified: true}
	for loader, contents := range config.Apps.TLS.Certificates {
		if loader != "automate" {
			return Policy{}, fmt.Errorf("manual certificate loaders require operator review")
		}
		if err := json.Unmarshal(contents, &policy.ManagedSubjects); err != nil {
			return Policy{}, err
		}
	}
	for _, automation := range config.Apps.TLS.Automation.Policies {
		if automation.OnDemand || len(automation.Managers) > 0 {
			return Policy{}, fmt.Errorf("dynamic certificate management requires operator review")
		}
	}
	for index, subject := range policy.ManagedSubjects {
		policy.ManagedSubjects[index] = normalizeName(subject)
		if !safeIdentifier(policy.ManagedSubjects[index]) {
			return Policy{}, fmt.Errorf("invalid runtime certificate subject")
		}
	}
	explicit := append([]string(nil), policy.ManagedSubjects...)
	for _, server := range config.Apps.HTTP.Servers {
		if len(server.TLS) == 0 {
			if !server.Automatic.Disabled {
				return Policy{}, fmt.Errorf("implicit TLS listeners require operator review")
			}
			continue
		}
		for _, connection := range server.TLS {
			if len(connection) != 0 {
				return Policy{}, fmt.Errorf("custom TLS connection policies require operator review")
			}
		}
		if len(server.Listen) == 0 {
			return Policy{}, fmt.Errorf("TLS listener address is missing")
		}
		for _, route := range server.Routes {
			if len(route.Match) == 0 {
				return Policy{}, fmt.Errorf("catch-all TLS routes require operator review")
			}
			for _, handler := range route.Handle {
				if handler.Handler == "subroute" {
					return Policy{}, fmt.Errorf("nested TLS routes require operator review")
				}
			}
			for _, match := range route.Match {
				if len(match.Host) == 0 {
					return Policy{}, fmt.Errorf("TLS route has no explicit hostname")
				}
				for _, host := range match.Host {
					host = normalizeName(host)
					if host == "" || strings.ContainsAny(host, "{} /\\") {
						return Policy{}, fmt.Errorf("invalid runtime certificate hostname")
					}
					for _, address := range server.Listen {
						listenHost, port, err := net.SplitHostPort(address)
						if err != nil {
							return Policy{}, fmt.Errorf("unsupported TLS listener: %s", address)
						}
						if listenHost == "" || listenHost == "0.0.0.0" {
							listenHost = "127.0.0.1"
						}
						if listenHost == "::" {
							listenHost = "::1"
						}
						if addressIP := net.ParseIP(listenHost); addressIP == nil || !addressIP.IsLoopback() {
							return Policy{}, fmt.Errorf("non-loopback TLS listener requires operator review")
						}
						policy.Endpoints = append(policy.Endpoints, Endpoint{Address: net.JoinHostPort(listenHost, port), Host: host})
					}
					if !server.Automatic.Disabled && !server.Automatic.DisableCertificates && !slices.Contains(server.Automatic.Skip, host) && !slices.Contains(server.Automatic.SkipCertificates, host) {
						policy.ManagedSubjects = append(policy.ManagedSubjects, host)
					}
				}
			}
		}
	}
	allSubjects := append([]string(nil), policy.ManagedSubjects...)
	policy.ManagedSubjects = nil
	for _, subject := range allSubjects {
		subject = normalizeName(subject)
		covered := false
		for _, wildcard := range allSubjects {
			if normalizeName(wildcard) != subject && Covers(wildcard, subject) {
				covered = true
			}
		}
		if !covered || slices.Contains(explicit, subject) {
			policy.ManagedSubjects = append(policy.ManagedSubjects, subject)
		}
	}
	slices.Sort(policy.ManagedSubjects)
	policy.ManagedSubjects = slices.Compact(policy.ManagedSubjects)
	return policy, nil
}

func Covers(subject, host string) bool {
	subject, host = normalizeName(subject), normalizeName(host)
	if subject == host {
		return true
	}
	if !strings.HasPrefix(subject, "*.") || strings.HasPrefix(host, "*.") {
		return false
	}
	suffix := strings.TrimPrefix(subject, "*")
	prefix, found := strings.CutSuffix(host, suffix)
	return found && prefix != "" && !strings.Contains(prefix, ".")
}

func Classify(snapshot *Snapshot, policy Policy) {
	snapshot.PolicyKnown = true
	for index := range snapshot.Certificates {
		status := &snapshot.Certificates[index]
		status.Usage = "unreferenced"
		status.CoveredBy = nil
		status.CanArchive = false
		for _, subject := range status.Subjects {
			for _, managed := range policy.ManagedSubjects {
				if Covers(subject, managed) {
					status.Usage = "managed"
				} else if Covers(managed, subject) {
					status.CoveredBy = append(status.CoveredBy, managed)
				}
			}
		}
		if status.Usage != "managed" && len(status.CoveredBy) > 0 {
			status.Usage = "covered"
		}
		slices.Sort(status.CoveredBy)
		status.CoveredBy = slices.Compact(status.CoveredBy)
		status.CanArchive = policy.verified && status.Usage != "managed" && len(status.Subjects) > 0 && len(snapshot.Warnings) == 0
	}
}

func normalizeName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}
