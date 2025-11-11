package app

import "strings"

func sanitizeID(raw string) string {
	if raw == "" {
		return ""
	}
	raw = strings.ToLower(raw)
	var b strings.Builder
	for _, r := range raw {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	return strings.Trim(b.String(), "_")
}

func serviceName(project, service, fallback string, includeProject bool) string {
	name := service
	if name == "" {
		name = fallback
	}
	name = sanitizeID(name)
	if includeProject {
		project = sanitizeID(project)
		if project != "" && name != "" {
			return project + "/" + name
		}
	}
	return name
}
