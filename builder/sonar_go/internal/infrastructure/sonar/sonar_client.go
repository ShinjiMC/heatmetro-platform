package sonar

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"heatmetro-go/internal/domain/metrics"
	"heatmetro-go/internal/domain/project"
)

const sonarHost = "https://sonarcloud.io"
const metricsQuery = "complexity,violations,coverage,ncloc,lines_to_cover"

// SonarClientAdapter implementa la interfaz ports.SonarClient
type SonarClientAdapter struct {
	Token      string
	HTTPClient *http.Client
}

// NewSonarClient crea una nueva instancia del cliente
func NewSonarClient(token string) *SonarClientAdapter {
	return &SonarClientAdapter{
		Token:      token,
		HTTPClient: &http.Client{},
	}
}

// Structs internos para parsear el JSON de la API de SonarCloud
type sonarBranchesResponse struct {
	Branches []struct {
		Name string `json:"name"`
	} `json:"branches"`
}

type sonarTreeResponse struct {
	BaseComponent sonarComponent   `json:"baseComponent"`
	Components    []sonarComponent `json:"components"`
}

type sonarComponent struct {
	Key       string         `json:"key"`
	Path      string         `json:"path"`
	Qualifier string         `json:"qualifier"`
	Measures  []sonarMeasure `json:"measures"`
}

type sonarMeasure struct {
	Metric string `json:"metric"`
	Value  string `json:"value"`
}

// CheckBranchExists verifica si una rama ya fue escaneada previamente
func (s *SonarClientAdapter) CheckBranchExists(projectKey, branchName string) (bool, error) {
	reqURL := fmt.Sprintf("%s/api/project_branches/list?project=%s", sonarHost, url.QueryEscape(projectKey))
	req, _ := http.NewRequest("GET", reqURL, nil)
	req.SetBasicAuth(s.Token, "")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, nil // Proyecto no existe o rama no encontrada
	}
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("error de API: HTTP %d", resp.StatusCode)
	}

	var data sonarBranchesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return false, err
	}

	for _, b := range data.Branches {
		if b.Name == branchName {
			return true, nil
		}
	}
	return false, nil
}

// RunScanner ejecuta el binario sonar-scanner a nivel de sistema
func (s *SonarClientAdapter) RunScanner(proj project.Project, branchName string) error {
	// Requiere que 'sonar-scanner' esté instalado y en el PATH del sistema
	cmd := exec.Command("sonar-scanner",
		"-Dsonar.projectKey="+proj.ProjectKey,
		"-Dsonar.branch.name="+branchName,
		"-Dsonar.token="+s.Token,
		"-Dsonar.host.url="+sonarHost,
	)
	cmd.Dir = proj.Path
	cmd.Env = append(os.Environ(), "SONAR_TOKEN="+s.Token)

	// Redirigimos la salida a la consola actual para poder monitorear
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("fallo al ejecutar sonar-scanner: %w", err)
	}
	
	// Limpieza de carpeta temporal como hacías en JS
	scannerWorkDir := filepath.Join(proj.Path, ".scannerwork")
	os.RemoveAll(scannerWorkDir)

	return nil
}

// RunScanner ejecuta el script de Node.js para mantener 100% de paridad con la versión JS original.
// func (s *SonarClientAdapter) RunScanner(proj project.Project, branchName string) error {
	
// 	// La ruta relativa hacia nuestro script de Node.js desde la raíz del proyecto Go
// 	scriptPath := filepath.Join("internal", "infrastructure", "sonar", "scripts", "run_scanner.js")
	
// 	// Verificamos que el script exista
// 	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
// 		return fmt.Errorf("no se encontró el script de Node en %s", scriptPath)
// 	}

// 	fmt.Println("--- Invocando SonarScanner a través de Node.js ---")

// 	// Invocamos Node: node run_scanner.js <repoPath> <branchName> <token>
// 	cmd := exec.Command("node", scriptPath, proj.Path, branchName, s.Token)
	
// 	// Redirigimos la salida a la consola actual para poder monitorearlo
// 	cmd.Stdout = os.Stdout
// 	cmd.Stderr = os.Stderr

// 	err := cmd.Run()
// 	if err != nil {
// 		return fmt.Errorf("fallo al ejecutar el script de Node.js para Sonar: %w", err)
// 	}
	
// 	// Limpieza de carpeta temporal (.scannerwork)
// 	scannerWorkDir := filepath.Join(proj.Path, ".scannerwork")
// 	os.RemoveAll(scannerWorkDir)

// 	return nil
// }

// ExtractMetrics descarga y procesa el JSON de métricas crudas
func (s *SonarClientAdapter) ExtractMetrics(projectKey, branchName string) ([]metrics.FileMetrics, []metrics.FolderMetrics, error) {
	reqURL := fmt.Sprintf("%s/api/measures/component_tree?component=%s&branch=%s&metricKeys=%s&qualifiers=FIL,DIR,TRK&ps=500",
		sonarHost, url.QueryEscape(projectKey), url.QueryEscape(branchName), metricsQuery)

	req, _ := http.NewRequest("GET", reqURL, nil)
	req.SetBasicAuth(s.Token, "")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("error obteniendo métricas HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var data sonarTreeResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, nil, err
	}

	allComponents := append(data.Components, data.BaseComponent)
	return s.parseSonarData(allComponents, projectKey, branchName)
}

// parseSonarData transforma los datos de Sonar en las entidades de métricas
func (s *SonarClientAdapter) parseSonarData(components []sonarComponent, projectKey, branchName string) ([]metrics.FileMetrics, []metrics.FolderMetrics, error) {
	var filesData []metrics.FileMetrics
	var foldersData []metrics.FolderMetrics

	for _, comp := range components {
		isRoot := comp.Qualifier == "TRK"
		path := comp.Path
		if isRoot {
			path = "/"
		}

		// Funciones de ayuda para parsear
		coverage := s.getFloatVal(comp.Measures, "coverage")
		complexity := s.getIntVal(comp.Measures, "complexity")
		violations := s.getIntVal(comp.Measures, "violations")
		ncloc := s.getIntVal(comp.Measures, "ncloc")
		linesToCover := s.getIntVal(comp.Measures, "lines_to_cover")

		if comp.Qualifier == "FIL" {
			finalCoverage := float64(0)

			isTestFile := strings.HasSuffix(path, "_test.go") || 
						  strings.HasSuffix(path, ".test.js") || 
						  strings.HasSuffix(path, ".spec.js") || 
						  strings.HasSuffix(path, ".test.ts") || 
						  strings.HasSuffix(path, ".spec.ts") ||
						  strings.HasSuffix(path, "Test.java") ||
						  strings.Contains(path, "src/test/") ||
						  strings.Contains(path, "src/tests/") ||
						  strings.Contains(path, "/test/") ||
						  strings.Contains(path, "/tests/")

			if isTestFile {
				finalCoverage = -1 // Excluido de deuda técnica
			} else if coverage != nil {
				finalCoverage = *coverage
			}

			fileMetric := metrics.FileMetrics{
				FilePath:   path,
				Type:       "FILE",
				Complexity: complexity,
				Sonar: metrics.SonarData{
					Coverage:     finalCoverage,
					LinesToCover: linesToCover,
					Issues:       violations,
					SonarURL:     s.buildSonarCodeUrl(comp.Key, projectKey, branchName),
					IssuesURL:    s.buildSonarIssuesUrl(comp.Key, false, projectKey, branchName),
				},
				Cohesion: metrics.Cohesion{
					LOC: ncloc,
				},
			}
			filesData = append(filesData, fileMetric)

		} else if comp.Qualifier == "DIR" || isRoot {
			avgCov := float64(0)
			if coverage != nil {
				avgCov = *coverage
			}

			folderMetric := metrics.FolderMetrics{
				FolderPath:      path,
				TotalIssues:     violations,
				TotalComplexity: complexity,
				AvgCoverage:     avgCov,
				SonarURL:        s.buildSonarCodeUrl(comp.Key, projectKey, branchName),
				IssuesURL:       s.buildSonarIssuesUrl(comp.Key, true, projectKey, branchName),
				// El resto de métricas se llenarán en la fase Bottom-Up
			}
			foldersData = append(foldersData, folderMetric)
		}
	}

	return filesData, foldersData, nil
}

func (s *SonarClientAdapter) getFloatVal(measures []sonarMeasure, key string) *float64 {
	for _, m := range measures {
		if m.Metric == key {
			if val, err := strconv.ParseFloat(m.Value, 64); err == nil {
				return &val
			}
		}
	}
	return nil
}

func (s *SonarClientAdapter) getIntVal(measures []sonarMeasure, key string) int {
	for _, m := range measures {
		if m.Metric == key {
			if val, err := strconv.Atoi(m.Value); err == nil {
				return val
			}
		}
	}
	return 0
}

func (s *SonarClientAdapter) buildSonarCodeUrl(componentKey, projectKey, branchName string) string {
	fullKey := componentKey
	if !strings.HasPrefix(fullKey, projectKey) {
		fullKey = projectKey + ":" + componentKey
	}
	return fmt.Sprintf("%s/code?id=%s&branch=%s&selected=%s", 
		sonarHost, url.QueryEscape(projectKey), url.QueryEscape(branchName), url.QueryEscape(fullKey))
}

func (s *SonarClientAdapter) buildSonarIssuesUrl(componentKey string, isFolder bool, projectKey, branchName string) string {
	baseParams := fmt.Sprintf("id=%s&branch=%s", url.QueryEscape(projectKey), url.QueryEscape(branchName))
	
	if isFolder {
		if componentKey == "/" {
			return fmt.Sprintf("%s/project/issues?%s&resolved=false", sonarHost, baseParams)
		}
		folderPath := strings.TrimPrefix(componentKey, projectKey+":")
		return fmt.Sprintf("%s/project/issues?%s&resolved=false&directories=%s", sonarHost, baseParams, url.QueryEscape(folderPath))
	}

	fullKey := componentKey
	if !strings.HasPrefix(fullKey, projectKey) {
		fullKey = projectKey + ":" + componentKey
	}
	return fmt.Sprintf("%s/code?%s&selected=%s", sonarHost, baseParams, url.QueryEscape(fullKey))
}