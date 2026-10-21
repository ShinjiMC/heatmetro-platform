package usecases

import (
	"fmt"
	"strings"
	"time"

	"heatmetro-go/internal/domain/criticality"
	"heatmetro-go/internal/domain/metrics"
	"heatmetro-go/internal/domain/ports"
	"heatmetro-go/internal/domain/project"
)

// ProjectProcessor orquesta todo el flujo de análisis. Reemplaza a analysis.runner.js y storage.service.js
type ProjectProcessor struct {
	db          ports.DBRepository
	gitCli      ports.GitClient
	sonar       ports.SonarClient
	critService *criticality.Service
}

func NewProjectProcessor(
	db ports.DBRepository,
	gitCli ports.GitClient,
	sonar ports.SonarClient,
	critService *criticality.Service,
) *ProjectProcessor {
	return &ProjectProcessor{
		db:          db,
		gitCli:      gitCli,
		sonar:       sonar,
		critService: critService,
	}
}


// RunFullAnalysis es el equivalente a processAndSaveMetrics y processSonarAnalysis
func (p *ProjectProcessor) RunFullAnalysis(proj project.Project, analyzer ports.AnalyzerStrategy, builder ports.BuildTool, onProgress func(int, string, string)) error {
	// Función auxiliar para emitir eventos de progreso si el callback existe
	reportProgress := func(pct int, step, msg string) {
		if onProgress != nil {
			onProgress(pct, step, msg)
		}
		fmt.Println(msg)
	}

	fmt.Printf("🚀 Iniciando Análisis Completo para: %s\n", proj.Path)

	// 1. Obtener información de Git
	reportProgress(10, "git", "Obteniendo información de Git...")
	sha, date, author, message, err := p.gitCli.GetLastCommitInfo(proj.Path)
	if err != nil {
		return fmt.Errorf("error obteniendo info de git: %w", err)
	}

	exists, err := p.db.CheckCommitExists(sha)
	if err != nil {
		return fmt.Errorf("error verificando BD: %w", err)
	}
	if exists {
		fmt.Println("✅ El commit ya está analizado en la BD. Retornando resultado inmediato.")
		return nil
	}

	hasChanges, _ := p.gitCli.HasCodeChanges(proj.Path, sha)
	if !hasChanges {
		fmt.Println("⏭️ Análisis omitido: No hay cambios en archivos de código.")
		return nil
	}

	// 2. Preparar el Entorno y Compilar (Reemplaza setup_environment y coverage)
	reportProgress(20, "compiling", "Preparando entorno y compilando...")
	err = builder.SetupEnvironment(proj)
	if err != nil {
		return fmt.Errorf("error configurando entorno: %w", err)
	}

	err = builder.Compile(proj)
	if err != nil {
		fmt.Printf("⚠️ Advertencia compilando: %v\n", err)
	}

	err = builder.GenerateCoverage(proj)
	if err != nil {
		fmt.Printf("⚠️ Advertencia generando coverage: %v\n", err)
	}

	// 3. Análisis de SonarQube (Reemplaza sonar.manager.js)
	fmt.Println("📡 Consultando SonarCloud...")
	reportProgress(35, "sonar", "Verificando rama en SonarCloud...")
	branchName := "commit-" + sha
	sonarExists, _ := p.sonar.CheckBranchExists(proj.ProjectKey, branchName)

	if !sonarExists {
		reportProgress(45, "sonar_scan", "Ejecutando SonarScanner (Esto puede tardar)...")
		err = p.sonar.RunScanner(proj, branchName)
		if err != nil {
			fmt.Printf("⚠️ Error en SonarScanner: %v\n", err)
		}
	} else {
		fmt.Println("✅ La rama YA existe en SonarCloud. Saltando subida.")
	}


	var sonarFiles []metrics.FileMetrics
	var sonarFolders []metrics.FolderMetrics

	// Loop de reintentos para asegurar que SonarCloud terminó de procesar
	maxAttempts := 10
	for i := 1; i <= maxAttempts; i++ {
		fmt.Printf("📥 Intento %d/%d... extrayendo métricas de SonarCloud.\n", i, maxAttempts)
		progressPct := 50 + (i * 2)
		reportProgress(progressPct, "sonar_extract", fmt.Sprintf("Extrayendo métricas de SonarCloud (Intento %d/%d)...", i, maxAttempts))
		sonarFiles, sonarFolders, err = p.sonar.ExtractMetrics(proj.ProjectKey, branchName)

		if err == nil && len(sonarFiles) > 0 {
			fmt.Printf("✅ Métricas obtenidas: %d archivos de Sonar.\n", len(sonarFiles))
			break
		}
		time.Sleep(5 * time.Second)
	}

	// 4. Analizadores Locales Matemáticos
	fmt.Println("🧮 Ejecutando Analizadores Locales (Halstead, Coupling, Layout, Churn)...")
	reportProgress(75, "local_metrics", "Ejecutando Analizadores Locales Matemáticos...")
	halsteadMap, _ := analyzer.CalculateHalstead(proj)
	couplingMap, _ := analyzer.CalculateCoupling(proj)
	layoutMap, cohesionMap, _ := analyzer.CalculateLayout(proj)

	var sinceDate *string
	totalCommits, _ := p.gitCli.GetTotalCommits(proj.Path)
	if totalCommits > 300 {
		limit := time.Now().AddDate(0, -9, 0).Format(time.RFC3339)
		sinceDate = &limit
		fmt.Printf("📅 > 300 commits. Limitando análisis de Churn a: %s\n", limit)
	}
	churnMap, _ := p.gitCli.GetFileChurnStream(proj.Path, sinceDate, proj.Exclusions)

	// 5. Consolidación de Datos en Memoria (Reemplaza fileMap de storage.service.js)
	reportProgress(85, "consolidation", "Consolidando métricas...")
	consolidatedFiles := p.mergeMetrics(sonarFiles, halsteadMap, couplingMap, layoutMap, cohesionMap, churnMap)

	// 6. Guardado en Base de Datos
	fmt.Printf("💾 Guardando datos para Commit: %s\n", sha)
	reportProgress(90, "database", "Guardando métricas en la base de datos...")
	shaID, err := p.db.SaveCommitInfo(sha, date, author, message)
	if err != nil {
		return fmt.Errorf("error guardando commit info: %w", err)
	}

	err = p.db.SaveFileMetrics(shaID, consolidatedFiles)
	if err != nil {
		return fmt.Errorf("error guardando file metrics: %w", err)
	}

	// 7. Cálculo y Guardado de Criticidad de Archivos (Severidad Absoluta/Relativa)
	var fileCriticalities []interface{}
	var typedFileCrits []criticality.FileCriticality // Lo necesitamos para el FolderAnalyzer

	for _, file := range consolidatedFiles {
		// CORRECCIÓN: Ignoramos los Structs y Paquetes para la tabla de Criticidad de Archivos
		// replicando la línea de SQL en JS: "AND (layout.type IS NULL OR layout.type != 'STRUCT')"
		// if file.Type == "STRUCT" || file.Type == "PACKAGE" {
		// 	continue
		// }

		if file.Type == "STRUCT" {
			continue
		}

		// Evaluamos la criticidad usando el Dominio (Valores por Ratio)
		crit := p.critService.CalculateFile(&file)
		fileCriticalities = append(fileCriticalities, crit)
		typedFileCrits = append(typedFileCrits, crit)
	}

	// 8. Cálculo de Jerarquía de Carpetas y Deuda Técnica
	folderAnalyzer := NewFolderAnalyzer(p.critService)
	folderMap, hierarchyMap := folderAnalyzer.BuildHierarchyMap(consolidatedFiles, sonarFolders)

	for path, folder := range folderMap {
		if isTestPath(path) {
			folder.AvgCoverage = -1
		} else {
			// Verificar si todos sus hijos son tests
			allTests := true
			children := hierarchyMap[path]
			if len(children) > 0 {
				for _, childName := range children {
					childPath := childName
					if path != "/" {
						childPath = path + "/" + childName
					}
					if !isTestPath(childPath) {
						allTests = false
						break
					}
				}
				if allTests {
					folder.AvgCoverage = -1
				}
			} else {
				allTests = false
			}
		}
	}

	// Aquí recuperamos las criticidades de las carpetas calculadas con el Bottom-Up
	reportProgress(95, "criticality", "Calculando criticidad final y jerarquías...")
	folderCrits := folderAnalyzer.CalculateFolderCriticality(folderMap, hierarchyMap, typedFileCrits, consolidatedFiles)

	// Anexamos las criticidades de las carpetas al array para que se guarden en tbl_file_criticality
	for _, fc := range folderCrits {
		fileCriticalities = append(fileCriticalities, fc)
	}

	// 9. GUARDADO UNIFICADO DE CRITICIDAD (Se guarda UNA SOLA VEZ)
	err = p.db.SaveCriticality(shaID, fileCriticalities)
	if err != nil {
		return fmt.Errorf("error guardando criticidad de archivos/carpetas: %w", err)
	}

	// Convertir el map de punteros a map de valores para la BD
	derefFolderMap := make(map[string]metrics.FolderMetrics)
	for path, stats := range folderMap {
		derefFolderMap[path] = *stats
	}

	// Guardar las métricas de carpetas
	err = p.db.SaveFolderMetrics(shaID, derefFolderMap)
	if err != nil {
		return fmt.Errorf("error guardando folder metrics: %w", err)
	}

	// Guardar la jerarquía relacional
	err = p.db.SaveFolderHierarchy(shaID, hierarchyMap)
	if err != nil {
		return fmt.Errorf("error guardando hierarchy: %w", err)
	}

	fmt.Println("🏁 Análisis finalizado y guardado con éxito.")
	reportProgress(100, "completed", "Análisis finalizado exitosamente.")
	return nil
}

func isTestPath(path string) bool {
	return strings.HasSuffix(path, "_test.go") ||
		strings.HasSuffix(path, ".test.js") ||
		strings.HasSuffix(path, ".spec.js") ||
		strings.HasSuffix(path, ".test.ts") ||
		strings.HasSuffix(path, ".spec.ts") ||
		strings.HasSuffix(path, "Test.java") ||
		strings.Contains(path, "src/test/") ||
		strings.Contains(path, "src/tests/") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/")
}

// mergeMetrics combina todos los mapas sueltos en un único slice de FileMetrics
func (p *ProjectProcessor) mergeMetrics(
	sonarFiles []metrics.FileMetrics,
	halstead map[string]metrics.Halstead,
	coupling map[string]metrics.Coupling,
	layout map[string]metrics.Layout,
	cohesion map[string]metrics.Cohesion,
	churn map[string]metrics.Churn,
) []metrics.FileMetrics {

	fileMap := make(map[string]*metrics.FileMetrics)

	// Inicializar con datos de Sonar
	for _, sf := range sonarFiles {
		copySf := sf // Copiar valor para evitar punteros al mismo elemento
		fileMap[sf.FilePath] = &copySf
	}

	// Helper para asegurar que la entrada exista
	getEntry := func(path string) *metrics.FileMetrics {
		if entry, exists := fileMap[path]; exists {
			return entry
		}
		newEntry := &metrics.FileMetrics{FilePath: path, Type: "FILE"}
		fileMap[path] = newEntry
		return newEntry
	}

	// Agregar Halstead
	for path, h := range halstead {
		getEntry(path).Halstead = h
	}

	// Agregar Coupling
	for path, c := range coupling {
		getEntry(path).Coupling = c
	}

	// Agregar Churn
	for path, ch := range churn {
		getEntry(path).Churn = ch
	}

	// Agregar Cohesion y Tipo (Package, Struct, File)
	for path, coh := range cohesion {
		entry := getEntry(path)
		entry.Cohesion = coh
		if coh.Type != "" {
			entry.Type = coh.Type
		}
	}

	// Agregar Layout
	for path, lay := range layout {
		getEntry(path).Layout = lay
	}

	// Normalizar y convertir a slice
	var result []metrics.FileMetrics
	for _, entry := range fileMap {
		if entry.Type == "" {
			entry.Type = "FILE"
		}
		if isTestPath(entry.FilePath) {
			entry.Sonar.Coverage = -1
		}
		result = append(result, *entry)
	}

	return result
}