package strategies

import (
	"fmt"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"heatmetro-go/internal/domain/metrics"
	"heatmetro-go/internal/domain/project"
	"heatmetro-go/internal/infrastructure/strategies/gocity"
	"heatmetro-go/internal/infrastructure/strategies/halstead"
)

type GoStrategy struct{}

func NewGoStrategy() *GoStrategy {
	return &GoStrategy{}
}

func isExcluded(path string, exclusions []string) bool {
	for _, pattern := range exclusions {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		cleanPattern := strings.ReplaceAll(pattern, "**", "")
		if cleanPattern != "" && strings.Contains(path, cleanPattern) {
			return true
		}
	}
	return false
}

// getGoModuleName lee el go.mod para obtener el prefijo exacto de los paquetes
func getGoModuleName(projectPath string) string {
	content, err := os.ReadFile(filepath.Join(projectPath, "go.mod"))
	if err == nil {
		re := regexp.MustCompile(`(?m)^module\s+(.*)$`)
		if match := re.FindStringSubmatch(string(content)); len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

func (s *GoStrategy) CalculateCoupling(proj project.Project) (map[string]metrics.Coupling, error) {
	fmt.Printf("--- Calculando Coupling Go (Nativo AST) en: %s ---\n", proj.Path)
	results := make(map[string]metrics.Coupling)
	globalImportsMap := make(map[string]int)

	modName := getGoModuleName(proj.Path)
	fset := token.NewFileSet()

	type fileInfo struct {
		Path    string
		PkgPath string
		FanOut  int
	}
	var files []fileInfo

	// FASE 1: Recorrer archivos y contar Fan-In / Fan-Out
	filepath.WalkDir(proj.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		relPath, _ := filepath.Rel(proj.Path, path)
		relPath = filepath.ToSlash(relPath)

		if isExcluded(relPath, proj.Exclusions) {
			return nil
		}

		// Parseo ultra rápido: Solo lee las líneas de import y se detiene.
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}

		fanOut := len(f.Imports)

		// Replicar comportamiento de `go list {{range .Imports}}`:
		// Solo los archivos NO-test aportan al Fan-In (Quién depende de mí)
		if !strings.HasSuffix(d.Name(), "_test.go") {
			for _, imp := range f.Imports {
				importPath := strings.Trim(imp.Path.Value, "\"")
				globalImportsMap[importPath]++
			}
		}

		// Calculamos la ruta absoluta del paquete (Ej: "github.com/sourcegraph/conc/pool")
		dir := filepath.Dir(relPath)
		pkgPath := modName
		if dir != "." && dir != "" {
			pkgPath = modName + "/" + filepath.ToSlash(dir)
		}

		files = append(files, fileInfo{Path: relPath, PkgPath: pkgPath, FanOut: fanOut})
		return nil
	})

	// FASE 2: Asignar a cada archivo el Fan-In de su paquete
	for _, f := range files {
		fanIn := globalImportsMap[f.PkgPath]
		results[f.Path] = metrics.Coupling{NumDependency: fanIn, NumImports: f.FanOut}
	}
	return results, nil
}

func round6(val float64) float64 {
	return math.Round(val*1e6) / 1e6
}

func (s *GoStrategy) CalculateHalstead(proj project.Project) (map[string]metrics.Halstead, error) {
	results := make(map[string]metrics.Halstead)
	fmt.Printf("--- Calculando Halstead Go (Nativo AsHTML) en: %s ---\n", proj.Path)

	processed := 0

	filepath.WalkDir(proj.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		relPath, _ := filepath.Rel(proj.Path, path)
		relPath = filepath.ToSlash(relPath)

		// Comentado temporalmente si los tests de exclusión te dan problemas, 
		// pero puedes descomentarlo cuando desees bloquear tests nativamente.
		// if isExcluded(relPath, proj.Exclusions) {
		// 	return nil
		// }

		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		// Usar la misma función que usaba tu CLI original
		operators, operands, totalOperators, totalOperands, err := halstead.AsHTML(src)
		if err != nil {
			return nil
		}

		differentOperands := len(operands)
		differentOperators := len(operators)
		programVocabulary := differentOperands + differentOperators
		programLength := totalOperands + totalOperators

		var volume, difficulty, effort, bugs float64

		if differentOperands > 0 && differentOperators > 0 {
			volume = float64(programLength) * math.Log2(float64(programVocabulary))
			difficulty = (float64(differentOperators) / 2.0) * (float64(totalOperands) / float64(differentOperands))
			effort = difficulty * volume
			bugs = math.Pow(effort, 2.0/3.0) / 3000.0
		}

		// Asignamos usando round6 para asegurar que mantenga los 6 decimales de precisión
		results[relPath] = metrics.Halstead{
			Volume:     round6(volume),
			Difficulty: round6(difficulty),
			Effort:     round6(effort),
			Bugs:       round6(bugs),
		}

		processed++
		return nil
	})

	fmt.Printf("Procesados %d archivos con Halstead exacto.\n", processed)
	return results, nil
}

func (s *GoStrategy) CalculateLayout(proj project.Project) (map[string]metrics.Layout, map[string]metrics.Cohesion, error) {
	layoutMap := make(map[string]metrics.Layout)
	cohesionMap := make(map[string]metrics.Cohesion)
	fmt.Printf("--- Calculando City Layout Go (Nativo Memoria) en: %s ---\n", proj.Path)

	// Inyectamos nuestra función de exclusión
	excludeFunc := func(path string) bool {
		return isExcluded(path, proj.Exclusions)
	}

	// 1. Ejecutar el Analyzer (Extrae datos de AST a Memoria)
	codeAnalyzer := gocity.NewAnalyzer(proj.Path, excludeFunc)
	summary, err := codeAnalyzer.Analyze()
	if err != nil {
		return layoutMap, cohesionMap, fmt.Errorf("error en analyzer: %w", err)
	}

	// 2. Construir el Árbol 3D de Gocity (RAÍZ VACÍA PARA EVITAR DOBLE SLASH)
	cityModel := gocity.New(summary, "")

	// 3. Aplanar el árbol y guardarlo en nuestros mapas
	var traverse func(node *gocity.Node)
	traverse = func(node *gocity.Node) {
		// Formateamos la ruta limpiando el slash inicial que deja Gocity al usar "" como raíz
		pathKey := strings.TrimPrefix(node.FullPath, "/")
		pathKey = strings.ReplaceAll(pathKey, "//", "/")

		// El nodo raíz general (el repositorio en sí) quedará vacío tras el Trim, lo forzamos a "/"
		if pathKey == "" {
			pathKey = "/"
		}

		if node.Type == gocity.FileType || node.Type == gocity.PackageType || node.Type == gocity.StructType {
			cW := node.ChildWidth
			cD := node.ChildDepth
			if pathKey == "/" {
				cW = 0.0
				cD = 0.0
			}

			layoutMap[pathKey] = metrics.Layout{
				RootW: node.Width, RootD: node.Depth,
				ChildW: cW, ChildD: cD,
				ChildX: node.ChildPosition.X, ChildY: node.ChildPosition.Y,
			}
			cohesionMap[pathKey] = metrics.Cohesion{
				Type:        string(node.Type),
				LOC:         node.NumberOfLines,
				MethodCount: node.NumberOfMethods,
				AttrCount:   node.NumberOfAttributes,
			}
		}

		for _, child := range node.Children {
			traverse(child)
		}
	}

	traverse(cityModel)

	return layoutMap, cohesionMap, nil
}