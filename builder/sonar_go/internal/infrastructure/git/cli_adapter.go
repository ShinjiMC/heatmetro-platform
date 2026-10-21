package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"heatmetro-go/internal/domain/metrics"
)

type CliAdapter struct{}

func NewCliAdapter() *CliAdapter {
	return &CliAdapter{}
}

// GetLastCommitInfo obtiene sha, date, author, message del HEAD
func (g *CliAdapter) GetLastCommitInfo(repoPath string) (sha, date, author, message string, err error) {
	cmd := exec.Command("git", "log", "-1", "--format=%H|%cI|%an|%s")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", "", "", "", fmt.Errorf("error obteniendo info de git: %w", err)
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 4)
	if len(parts) == 4 {
		// CORRECCIÓN DE FECHA: Forzar el formato a UTC con 'Z' exacto de JavaScript
		parsedDate, parseErr := time.Parse(time.RFC3339, parts[1])
		finalDate := parts[1]
		if parseErr == nil {
			// Format "2006-01-02T15:04:05Z" es el equivalente exacto a .toISOString() en JS
			finalDate = parsedDate.UTC().Format("2006-01-02T15:04:05Z")
		}
		
		return parts[0], finalDate, parts[2], parts[3], nil
	}
	return "unknown", "unknown", "unknown", "unknown", nil
}

func (g *CliAdapter) GetTotalCommits(repoPath string) (int, error) {
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return count, nil
}

func (g *CliAdapter) CheckoutRepo(repoPath string, sha string) error {
	cmd := exec.Command("git", "checkout", "-f", sha)
	cmd.Dir = repoPath
	return cmd.Run()
}

func (g *CliAdapter) HasCodeChanges(repoPath string, sha string) (bool, error) {
	cmd := exec.Command("git", "show", "--name-only", "--format=", sha)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return true, err // En caso de duda (ej. primer commit), retornamos true
	}

	codeExtensions := []string{".go", ".ts", ".js", ".java", ".py", ".rb", ".cpp", ".c", ".cs"}
	lines := strings.Split(string(out), "\n")

	for _, line := range lines {
		file := strings.TrimSpace(line)
		for _, ext := range codeExtensions {
			if strings.HasSuffix(file, ext) {
				return true, nil
			}
		}
	}
	return false, nil
}

// GetFileChurnStream reemplaza a churn.js y churn_java.js de un solo golpe
func (g *CliAdapter) GetFileChurnStream(repoPath string, sinceDate *string, exclusions []string) (map[string]metrics.Churn, error) {
	metricsMap := make(map[string]*metrics.Churn)
	
	// 1. Obtener archivos actuales rastreados por git
	cmdLs := exec.Command("git", "ls-files", "--exclude-standard")
	cmdLs.Dir = repoPath
	outLs, _ := cmdLs.Output()
	
	scanner := bufio.NewScanner(bytes.NewReader(outLs))
	for scanner.Scan() {
		file := strings.TrimSpace(scanner.Text())
		if file == "" || g.isExcluded(file, exclusions) {
			continue
		}
		// Solo nos interesan código fuente principal
		if strings.HasSuffix(file, ".go") || strings.HasSuffix(file, ".java") {
			metricsMap[file] = &metrics.Churn{}
		}
	}

	if len(metricsMap) == 0 {
		return make(map[string]metrics.Churn), nil
	}

	// 2. Historial de cambios (numstat)
	args := []string{"log", "--numstat", "--pretty=format:###|%H|%ae"}
	if sinceDate != nil {
		args = append(args, "--since="+*sinceDate)
	}
	// Analizamos historia de Go y Java al mismo tiempo
	args = append(args, "--", "*.go", "*.java")

	cmdLog := exec.Command("git", args...)
	cmdLog.Dir = repoPath
	outLog, _ := cmdLog.Output()

	scannerLog := bufio.NewScanner(bytes.NewReader(outLog))
	
	// Helper auxiliar para contar autores sin usar SETs pesados
	authorsMap := make(map[string]map[string]bool)

	var currentAuthor string
	for scannerLog.Scan() {
		line := strings.TrimSpace(scannerLog.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "###|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				currentAuthor = parts[2]
			}
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		file := parts[2]
		stats, exists := metricsMap[file]
		if !exists || g.isExcluded(file, exclusions) {
			continue
		}

		addRaw, delRaw := parts[0], parts[1]
		if addRaw != "-" {
			adds, _ := strconv.Atoi(addRaw)
			stats.Added += adds
		}
		if delRaw != "-" {
			dels, _ := strconv.Atoi(delRaw)
			stats.Deleted += dels
		}
		
		stats.Frequency++
		
		if currentAuthor != "" {
			if authorsMap[file] == nil {
				authorsMap[file] = make(map[string]bool)
			}
			authorsMap[file][currentAuthor] = true
		}
	}

	// 3. Empaquetar resultado
	finalResults := make(map[string]metrics.Churn)
	for file, stats := range metricsMap {
		stats.Total = stats.Added + stats.Deleted
		stats.Authors = len(authorsMap[file])
		finalResults[file] = *stats
	}

	return finalResults, nil
}

func (g *CliAdapter) isExcluded(path string, exclusions []string) bool {
	for _, pattern := range exclusions {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Aproximación simple a wildcard de carpetas
		if strings.Contains(path, strings.ReplaceAll(pattern, "**", "")) {
			return true
		}
	}
	return false
}