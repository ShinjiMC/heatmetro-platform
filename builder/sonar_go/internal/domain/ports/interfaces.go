package ports

import (
	// Nota: Si inicializaste tu módulo con otro nombre, cambia "heatmetro-go"
	"heatmetro-go/internal/domain/metrics"
	"heatmetro-go/internal/domain/project"
)


// DBRepository es el contrato que implementará sqlite_adapter.go.
// El dominio no sabe que usamos SQLite, solo sabe que puede guardar y leer.
// Reemplaza a las consultas de database.js.
type DBRepository interface {
	CheckCommitExists(sha string) (bool, error)
	SaveCommitInfo(sha, date, author, message string) (int64, error)
	SaveFileMetrics(shaID int64, files []metrics.FileMetrics) error
	SaveFolderMetrics(shaID int64, folders map[string]metrics.FolderMetrics) error
	SaveCriticality(shaID int64, criticalities []interface{}) error // Recibirá un struct de criticidad
	SaveFolderHierarchy(shaID int64, hierarchy map[string][]string) error
}

// GitClient abstrae las llamadas a consola de os/exec("git").
// Reemplaza a git.service.js.
type GitClient interface {
	GetLastCommitInfo(path string) (sha, date, author, message string, err error)
	GetTotalCommits(path string) (int, error)
	HasCodeChanges(path string, sha string) (bool, error)
	CheckoutRepo(path string, sha string) error
	GetFileChurnStream(path string, sinceDate *string, exclusions []string) (map[string]metrics.Churn, error)
}

// AnalyzerStrategy es la obra maestra de SOLID.
// En lugar de tener ifs gigantes, GoStrategy y JavaStrategy implementarán esta interfaz.
type AnalyzerStrategy interface {
	CalculateHalstead(proj project.Project) (map[string]metrics.Halstead, error)
	CalculateCoupling(proj project.Project) (map[string]metrics.Coupling, error)
	CalculateLayout(proj project.Project) (map[string]metrics.Layout, map[string]metrics.Cohesion, error)
}


// SonarClient se encarga de hablar con SonarCloud.
type SonarClient interface {
	CheckBranchExists(projectKey, branchName string) (bool, error)
	RunScanner(proj project.Project, branchName string) error
	ExtractMetrics(projectKey, branchName string) ([]metrics.FileMetrics, []metrics.FolderMetrics, error)
}

// BuildTool representa el compilador (Maven, Gradle, Go Build).
// Aquí es donde inyectaremos el JDK dinámico en Java o el GOROOT en Go.
type BuildTool interface {
	SetupEnvironment(proj project.Project) error
	Compile(proj project.Project) error
	GenerateCoverage(proj project.Project) error
}

// JdkManager es tu solución al problema corporativo de Java.
// Se encargará de buscar, mapear o descargar las versiones exactas de Java.
type JdkManager interface {
	GetJavaHome(requiredVersion string) (string, error)
}