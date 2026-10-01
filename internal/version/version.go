package version

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	Dirty     = "unknown"
)

type Metadata struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	Dirty     string `json:"dirty"`
}

func Info() Metadata {
	return Metadata{
		Version:   valueOrDefault(Version, "dev"),
		Commit:    valueOrDefault(Commit, "unknown"),
		BuildDate: valueOrDefault(BuildDate, "unknown"),
		Dirty:     valueOrDefault(Dirty, "unknown"),
	}
}

func (m Metadata) FormatHuman() string {
	return fmt.Sprintf("version: %s\ncommit: %s\nbuild_date: %s\ndirty: %s\n", m.Version, m.Commit, m.BuildDate, m.Dirty)
}

var unsafeArtifactChar = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func SanitizeForArtifact(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	return unsafeArtifactChar.ReplaceAllString(version, "_")
}

func valueOrDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
