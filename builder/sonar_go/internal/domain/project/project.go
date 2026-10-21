package project

import (
	"os"
	"path/filepath"
)

// Project encapsula la información básica del repositorio a analizar
type Project struct {
	Path       string
	ProjectKey string
	Exclusions []string
	Language   string // "JAVA", "GO", o "UNKNOWN"
}

// DetectLanguage reemplaza a select_language.js
func DetectLanguage(repoPath string) string {
	pomPath := filepath.Join(repoPath, "pom.xml")
	gradlePath := filepath.Join(repoPath, "build.gradle")
	goModPath := filepath.Join(repoPath, "go.mod")

	if _, err := os.Stat(pomPath); err == nil {
		return "JAVA"
	}
	if _, err := os.Stat(gradlePath); err == nil {
		return "JAVA"
	}
	if _, err := os.Stat(goModPath); err == nil {
		return "GO"
	}

	return "UNKNOWN"
}