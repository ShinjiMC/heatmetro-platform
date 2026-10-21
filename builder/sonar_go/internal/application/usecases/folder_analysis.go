package usecases

import (
	"math"
	"path/filepath"
	"sort"
	"strings"

	"heatmetro-go/internal/domain/criticality"
	"heatmetro-go/internal/domain/metrics"
)

// FolderAnalyzer reemplaza hierarchy.js y calculateAndSaveFolderCriticality
type FolderAnalyzer struct {
	critService *criticality.Service
}

func NewFolderAnalyzer(cs *criticality.Service) *FolderAnalyzer {
	return &FolderAnalyzer{critService: cs}
}

type debtAccumulator struct {
	WeightedSum float64
	TotalLines  float64
}

// Mapas de conversión equivalentes a VALS_TO_SEVERITY de Node.js
var severitiesToInt = map[criticality.Severity]int{
	criticality.Low:      1,
	criticality.Medium:   2,
	criticality.High:     3,
	criticality.Critical: 4,
}

var intToSeverity = map[int]criticality.Severity{
	1: criticality.Low,
	2: criticality.Medium,
	3: criticality.High,
	4: criticality.Critical,
}


// BuildHierarchyMap recorre los archivos y sube por sus directorios padres para sumar métricas (Bottom-Up)
func (f *FolderAnalyzer) BuildHierarchyMap(files []metrics.FileMetrics, sonarFolders []metrics.FolderMetrics) (map[string]*metrics.FolderMetrics, map[string][]string) {
	folderMap := make(map[string]*metrics.FolderMetrics)
	hierarchyMap := make(map[string][]string)

	// 1. Inicializamos las carpetas SOLO con el Coverage de Sonar.
	for _, sf := range sonarFolders {
		folderMap[sf.FolderPath] = &metrics.FolderMetrics{
			FolderPath:  sf.FolderPath,
			AvgCoverage: sf.AvgCoverage,
			SonarURL:    sf.SonarURL,
			IssuesURL:   sf.IssuesURL,
		}
	}

	for _, file := range files {
		isStruct := file.Type == "STRUCT"
		isPackage := file.Type == "PACKAGE"
		isFile := !isStruct && !isPackage

		normalizedPath := strings.ReplaceAll(file.FilePath, "\\", "/")

		if isPackage {
			if normalizedPath == "" || normalizedPath == "." {
				normalizedPath = "/"
			}
			if _, exists := folderMap[normalizedPath]; !exists {
				folderMap[normalizedPath] = &metrics.FolderMetrics{FolderPath: normalizedPath}
			}
			stats := folderMap[normalizedPath]

			realLoc := file.Cohesion.LOC
			if realLoc == 0 {
				realLoc = file.Churn.Total
			}
			stats.TotalLOC = realLoc
			stats.TotalFuncCount = file.Cohesion.AttrCount
			stats.TotalMethodCount = file.Cohesion.MethodCount
			continue
		}

		if !isFile {
			continue
		}

		currentPath := filepath.Dir(normalizedPath)
		currentPath = strings.ReplaceAll(currentPath, "\\", "/")
		childName := filepath.Base(normalizedPath)

		if currentPath == "." || currentPath == "" {
			currentPath = "/"
		}

		for {
			if _, exists := folderMap[currentPath]; !exists {
				folderMap[currentPath] = &metrics.FolderMetrics{FolderPath: currentPath}
			}
			stats := folderMap[currentPath]

			// Acumulación pura Bottom-Up
			stats.TotalIssues += file.Sonar.Issues
			stats.TotalComplexity += file.Complexity
			stats.TotalChurn += file.Churn.Total
			stats.TotalCouplingDeps += file.Coupling.NumDependency
			stats.TotalFrequency += file.Churn.Frequency
			stats.TotalAuthors += file.Churn.Authors
			stats.TotalHalsteadVolume += file.Halstead.Volume
			stats.TotalHalsteadDiff += file.Halstead.Difficulty
			stats.TotalHalsteadEffort += file.Halstead.Effort
			stats.TotalHalsteadBugs += file.Halstead.Bugs

			hierarchyMap[currentPath] = appendUnique(hierarchyMap[currentPath], childName)

			if currentPath == "/" {
				break
			}

			childName = filepath.Base(currentPath)
			currentPath = filepath.Dir(currentPath)
			currentPath = strings.ReplaceAll(currentPath, "\\", "/")
			if currentPath == "." {
				currentPath = "/"
			}
		}
	}

	return folderMap, hierarchyMap
}


// CalculateFolderCriticality evalúa la deuda de carpetas de adentro hacia afuera (Bottom-Up)
func (f *FolderAnalyzer) CalculateFolderCriticality(
	folderMap map[string]*metrics.FolderMetrics,
	hierarchyMap map[string][]string,
	fileCrits []criticality.FileCriticality,
	files []metrics.FileMetrics,
) []criticality.FileCriticality {

	// 1. Mapas rápidos (O(1) lookups)
	fileCritMap := make(map[string]criticality.FileCriticality)
	for _, fc := range fileCrits {
		fileCritMap[fc.FilePath] = fc
	}

	fileMetricsMap := make(map[string]metrics.FileMetrics)
	for _, fm := range files {
		fileMetricsMap[fm.FilePath] = fm
	}

	// 2. Ordenar rutas desde el nivel más profundo hasta la raíz ("/")
	var sortedPaths []string
	for p := range folderMap {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Slice(sortedPaths, func(i, j int) bool {
		return getDepth(sortedPaths[i]) > getDepth(sortedPaths[j])
	})

	var folderCriticalities []criticality.FileCriticality

	// 3. Iteración Bottom-Up
	for _, folderPath := range sortedPaths {
		children := hierarchyMap[folderPath]
		currentFolder := folderMap[folderPath]

		// Estado inicial: Severidad Máxima en LOW (1), Acumuladores de deuda en 0
		maxSev := map[string]int{
			"complexity": 1, "coupling": 1, "churn": 1,
			"authors": 1, "overall": 1,
		}

		debtAcc := map[string]*debtAccumulator{
			"complexity": {}, "coupling": {}, "churn": {},
			"authors": {}, "overall": {},
		}

		for _, childName := range children {
			childPath := childName
			if folderPath != "/" {
				childPath = folderPath + "/" + childName
			}

			if childFolder, isFolder := folderMap[childPath]; isFolder {
				for mKey, childValString := range map[string]string{
					"complexity": childFolder.SeverityComplexity,
					"coupling":   childFolder.SeverityCoupling,
					"churn":      childFolder.SeverityChurn,
					"authors":    childFolder.SeverityAuthors,
					"overall":    childFolder.OverallSeverity,
				} {
					val := severitiesToInt[criticality.Severity(childValString)]
					if val > maxSev[mKey] {
						// maxSev[mKey] = val //REVISE
						// fmt.Printf("   📁 Subcarpeta '%s' elevó '%s' de la carpeta '%s' a %s\n", childPath, mKey, folderPath, childValString)
					}
				}
				f.updateMaxSeverityFromFolder(maxSev, childFolder)

				metricsKeys := []string{"overall", "complexity", "coupling", "churn", "authors"}
				for _, mKey := range metricsKeys {
					var debtValue *float64
					switch mKey {
					case "overall": debtValue = childFolder.DebtCovOverall
					case "complexity": debtValue = childFolder.DebtCovComplexity
					case "coupling": debtValue = childFolder.DebtCovCoupling
					case "churn": debtValue = childFolder.DebtCovChurn
					case "authors": debtValue = childFolder.DebtCovAuthors
					}

					if debtValue != nil && childFolder.TotalLOC > 0 {
						debtAcc[mKey].WeightedSum += (*debtValue / 100.0) * float64(childFolder.TotalLOC)
						debtAcc[mKey].TotalLines += float64(childFolder.TotalLOC)
					}
				}

			} else if fileCrit, isFile := fileCritMap[childPath]; isFile {
				// Si el hijo es un ARCHIVO
				fm := fileMetricsMap[childPath]
				weight := float64(fm.Sonar.LinesToCover)
				coverage := fm.Sonar.Coverage

				if coverage == -1 {
					weight = 0 // Coverage = -1 indica archivo Test (No acumula deuda)
				}

				metricsMap := map[string]criticality.Severity{
					"complexity": fileCrit.SeverityComplexity,
					"coupling":   fileCrit.SeverityCoupling,
					"churn":      fileCrit.SeverityChurn,
					"authors":    fileCrit.SeverityAuthors,
					"overall":    fileCrit.OverallSeverity,
				}

				for key, sev := range metricsMap {
					val := severitiesToInt[sev]
					if val > maxSev[key] {
						maxSev[key] = val
					}

					if val >= 3 && weight > 0 {
						acc := debtAcc[key]
						acc.WeightedSum += weight * (coverage / 100.0)
						acc.TotalLines += weight
					}
				}
			}
		}


		// Asignar severidades máximas
		currentFolder.SeverityComplexity = string(intToSeverity[maxSev["complexity"]])
		currentFolder.SeverityCoupling = string(intToSeverity[maxSev["coupling"]])
		currentFolder.SeverityChurn = string(intToSeverity[maxSev["churn"]])
		currentFolder.SeverityAuthors = string(intToSeverity[maxSev["authors"]])
		currentFolder.OverallSeverity = string(intToSeverity[maxSev["overall"]])

		// Asignar deuda calculada
		currentFolder.DebtCovOverall = calculateFinalDebt(debtAcc["overall"])
		currentFolder.DebtCovComplexity = calculateFinalDebt(debtAcc["complexity"])
		currentFolder.DebtCovCoupling = calculateFinalDebt(debtAcc["coupling"])
		currentFolder.DebtCovChurn = calculateFinalDebt(debtAcc["churn"])
		currentFolder.DebtCovAuthors = calculateFinalDebt(debtAcc["authors"])

		folderCriticalities = append(folderCriticalities, criticality.FileCriticality{
			FilePath:           folderPath,
			SeverityComplexity: intToSeverity[maxSev["complexity"]],
			SeverityCoupling:   intToSeverity[maxSev["coupling"]],
			SeverityChurn:      intToSeverity[maxSev["churn"]],
			SeverityAuthors:    intToSeverity[maxSev["authors"]],
			OverallSeverity:    intToSeverity[maxSev["overall"]],
		})
	}
	return folderCriticalities
}

// getDepth cuenta la profundidad del path para ordenar Bottom-Up
func getDepth(p string) int {
	if p == "/" {
		return 0
	}
	return strings.Count(p, "/") + 1
}

// appendUnique evita hijos duplicados en la jerarquía
func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	slice = append(slice, item)
	sort.Strings(slice)
	return slice
}

// calculateFinalDebt realiza el promedio ponderado de la deuda (WeightedSum / TotalLines) * 100
func calculateFinalDebt(acc *debtAccumulator) *float64 {
	if acc.TotalLines > 0 {
		rawVal := (acc.WeightedSum / acc.TotalLines) * 100.0
		rounded := math.Round(rawVal*10) / 10
		return &rounded
	}
	return nil 
}

// updateMaxSeverityFromFolder hereda la severidad máxima de la subcarpeta
func (f *FolderAnalyzer) updateMaxSeverityFromFolder(maxSev map[string]int, folder *metrics.FolderMetrics) {
	if severitiesToInt[criticality.Severity(folder.SeverityComplexity)] > maxSev["complexity"] {
		maxSev["complexity"] = severitiesToInt[criticality.Severity(folder.SeverityComplexity)]
	}
	if severitiesToInt[criticality.Severity(folder.SeverityCoupling)] > maxSev["coupling"] {
		maxSev["coupling"] = severitiesToInt[criticality.Severity(folder.SeverityCoupling)]
	}
	if severitiesToInt[criticality.Severity(folder.SeverityChurn)] > maxSev["churn"] {
		maxSev["churn"] = severitiesToInt[criticality.Severity(folder.SeverityChurn)]
	}
	if severitiesToInt[criticality.Severity(folder.SeverityAuthors)] > maxSev["authors"] {
		maxSev["authors"] = severitiesToInt[criticality.Severity(folder.SeverityAuthors)]
	}
	if severitiesToInt[criticality.Severity(folder.OverallSeverity)] > maxSev["overall"] {
		maxSev["overall"] = severitiesToInt[criticality.Severity(folder.OverallSeverity)]
	}
}