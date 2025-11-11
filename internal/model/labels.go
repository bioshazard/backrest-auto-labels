package model

import (
	"strings"
)

// GetLabel returns trimmed value or default.
func GetLabel(labels map[string]string, key, def string) string {
	if labels == nil {
		return def
	}
	if v, ok := labels[key]; ok {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return def
}

// BoolLabel returns true if the label value equals "true" (case-insensitive).
func BoolLabel(labels map[string]string, key string) bool {
	if labels == nil {
		return false
	}
	val, ok := labels[key]
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "t", "true", "y", "yes":
		return true
	default:
		return false
	}
}

// ComposeMetadata extracts project/service names from labels.
func ComposeMetadata(labels map[string]string) (project, service string) {
	if labels == nil {
		return "", ""
	}
	project = strings.TrimSpace(labels[LabelComposeProject])
	service = strings.TrimSpace(labels[LabelComposeService])
	return project, service
}
