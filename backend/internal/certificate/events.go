package certificate

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/aidockerfarm/gateway/internal/logs"
)

type Event struct {
	Time      time.Time  `json:"time"`
	Operation string     `json:"operation"`
	Outcome   string     `json:"outcome"`
	Subjects  []string   `json:"subjects"`
	ErrorCode string     `json:"errorCode,omitempty"`
	RetryAt   *time.Time `json:"retryAt,omitempty"`
}

func RecentEvents(entries []logs.Entry) []Event {
	events := []Event{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Source, "caddy/") {
			continue
		}
		operation := ""
		switch {
		case strings.HasSuffix(entry.Source, "/tls.renew"):
			operation = "renew"
		case strings.HasSuffix(entry.Source, "/tls.obtain"):
			operation = "obtain"
		default:
			continue
		}
		event := Event{Time: entry.Time, Operation: operation, Subjects: []string{}}
		switch {
		case entry.Message == "certificate renewed successfully" || entry.Message == "certificate obtained successfully":
			event.Outcome = "success"
		case entry.Message == "will retry":
			event.Outcome = "retry"
		case entry.Level == "error":
			event.Outcome = "failure"
		case entry.Message == "renewing certificate" || entry.Message == "obtaining certificate":
			event.Outcome = "started"
		default:
			continue
		}
		if identifier, ok := entry.Fields["identifier"].(string); ok && safeIdentifier(identifier) {
			event.Subjects = append(event.Subjects, identifier)
		}
		if identifiers, ok := entry.Fields["identifiers"].([]any); ok {
			for _, identifier := range identifiers {
				if name, ok := identifier.(string); ok && safeIdentifier(name) {
					event.Subjects = append(event.Subjects, name)
				}
			}
		}
		if event.Outcome == "failure" || event.Outcome == "retry" {
			detail := strings.ToLower(fmt.Sprint(entry.Fields["error"]))
			if len(event.Subjects) == 0 && strings.HasPrefix(detail, "[") {
				if identifier, remainder, found := strings.Cut(detail[1:], "] "); found && safeIdentifier(identifier) && (strings.HasPrefix(remainder, "renew:") || strings.HasPrefix(remainder, "obtain:")) {
					event.Subjects = append(event.Subjects, identifier)
				}
			}
			switch {
			case strings.Contains(detail, "rate") && strings.Contains(detail, "limit"):
				event.ErrorCode = "rate_limit"
			case strings.Contains(detail, "unauthorized") || strings.Contains(detail, "403") || strings.Contains(detail, "credential"):
				event.ErrorCode = "authorization"
			case strings.Contains(detail, "dns") || strings.Contains(detail, "propagation"):
				event.ErrorCode = "dns"
			case strings.Contains(detail, "timeout") || strings.Contains(detail, "connection"):
				event.ErrorCode = "network"
			default:
				event.ErrorCode = "other"
			}
		}
		if seconds, ok := entry.Fields["retrying_in"].(float64); ok && seconds >= 0 && seconds < 86400*365 && !math.IsNaN(seconds) {
			retry := entry.Time.Add(time.Duration(seconds * float64(time.Second)))
			event.RetryAt = &retry
		}
		events = append(events, event)
		if len(events) == 50 {
			break
		}
	}
	return events
}

func safeIdentifier(value string) bool {
	if value == "" || len(value) > 253 {
		return false
	}
	for _, character := range value {
		if character != '.' && character != '-' && character != '*' && character != ':' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}
