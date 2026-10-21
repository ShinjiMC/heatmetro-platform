package gocity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

// Analizador expone el método para generar el resumen de nodos en memoria.
type Analyzer struct {
	rootPath    string
	excludeFunc func(string) bool
}

// NewAnalyzer inyecta la función de exclusión de nuestro Dominio para no depender de librerías externas.
func NewAnalyzer(rootPath string, excludeFunc func(string) bool) *Analyzer {
	return &Analyzer{
		rootPath:    rootPath,
		excludeFunc: excludeFunc,
	}
}

func (a *Analyzer) IsInvalidPath(absolutePath string) bool {
	relPath, err := filepath.Rel(a.rootPath, absolutePath)
	if err != nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	
	// Delegamos la verificación a la función inyectada por GoStrategy
	if a.excludeFunc != nil {
		return a.excludeFunc(relPath)
	}
	return false
}

func (a *Analyzer) Analyze() (map[string]*NodeInfo, error) {
	summary := make(map[string]*NodeInfo)
	err := filepath.Walk(a.rootPath, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error recorriendo archivo: %w", err)
		}

		fileSet := token.NewFileSet()
		if f.IsDir() || !IsGoFile(f.Name()) || a.IsInvalidPath(path) {
			return nil
		}
		
		relPath, err := filepath.Rel(a.rootPath, path)
		if err != nil {
			relPath = path
		}
		relPath = filepath.ToSlash(relPath)

		// Parseo de AST en Memoria
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return nil // Saltamos archivos malformados
		}

		v := &Visitor{
			FileSet:    fileSet,
			Path:       relPath,
			StructInfo: summary,
		}

		ast.Walk(v, file)
		return nil
	})

	return summary, err
}