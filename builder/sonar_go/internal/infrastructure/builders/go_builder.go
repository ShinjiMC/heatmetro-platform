package builders

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"heatmetro-go/internal/domain/project"
)

// Define aquí la versión base de Go a la que tu máquina debe regresar
// después de compilar el proyecto a analizar.
const baseGoVersion = "1.25.0" 

// GoBuilder implementa ports.BuildTool para proyectos Go
type GoBuilder struct{
	targetVersion string
}

func NewGoBuilder() *GoBuilder {
	return &GoBuilder{}
}

func (b *GoBuilder) SetupEnvironment(proj project.Project) error {
	fmt.Printf("Configurando entorno Go en: %s\n", proj.Path)

	goModFile := filepath.Join(proj.Path, "go.mod")
	content, err := os.ReadFile(goModFile)
	if err != nil {
		return fmt.Errorf("no se encontró go.mod: %w", err)
	}

	re := regexp.MustCompile(`(?m)^go\s+(\d+(\.\d+)?(\.\d+)?)`)
	match := re.FindStringSubmatch(string(content))
	if len(match) < 2 {
		return fmt.Errorf("no se pudo detectar la versión en go.mod")
	}

	reqVersion := match[1]
	fmt.Printf("Versión de Go requerida en go.mod: %s\n", reqVersion)

	// 1. Forzar instalación con 'g' (ej: 'g install 1.12' bajará la última 1.12.x)
	fmt.Printf("Asegurando instalación de Go %s con 'g'...\n", reqVersion)
	cmdInstall := exec.Command("g", "install", reqVersion)
	cmdInstall.Stdout = os.Stdout
	cmdInstall.Stderr = os.Stderr
	_ = cmdInstall.Run()

	exactVersion, err := b.findInstalledVersion(reqVersion)
	if err != nil || exactVersion == "" {
		exactVersion = reqVersion
	}
	
	b.targetVersion = exactVersion
	fmt.Printf("Versión exacta de Go lista para usar: %s\n", b.targetVersion)

	return nil
}

func (b *GoBuilder) findInstalledVersion(reqVersion string) (string, error) {
	cmdCheck := exec.Command("g", "ls")
	out, err := cmdCheck.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		clean = strings.TrimPrefix(clean, "*")
		clean = strings.TrimSpace(clean)

		if clean == reqVersion || strings.HasPrefix(clean, reqVersion+".") {
			return clean, nil
		}
	}
	return reqVersion, nil
}

// switchGoVersion cambia la versión activa del sistema usando la herramienta 'g'
func (b *GoBuilder) switchGoVersion(version string) error {
	fmt.Printf("   [Go Switch] Cambiando globalmente a Go %s...\n", version)
	cmd := exec.Command("g", "use", version)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("falló 'g use %s': %s", version, string(out))
	}
	return nil
}

func (b *GoBuilder) Compile(proj project.Project) error {
	// 1. Forzar versión del proyecto (Ej: g use 1.20)
	err := b.switchGoVersion(b.targetVersion)
	if err != nil {
		fmt.Printf("⚠️ Falló switch a Go %s. Compilando con versión actual.\n", b.targetVersion)
	} else {
		defer b.switchGoVersion(baseGoVersion)
	}

	fmt.Println("   👉 Limpiando caché y compilando proyecto Go (go build ./...)...")
	env := append(os.Environ(), "GOTOOLCHAIN=local")
	
	// Limpieza obligatoria cruzada para evitar el error "compile: version does not match"
	cmdClean := exec.Command("go", "clean", "-cache")
	cmdClean.Dir = proj.Path
	cmdClean.Env = env
	_ = cmdClean.Run()

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = proj.Path
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error compilando Go: %s", string(out))
	}
	return nil
}

func (b *GoBuilder) GenerateCoverage(proj project.Project) error {
	// 1. Forzar versión del proyecto para los tests
	err := b.switchGoVersion(b.targetVersion)
	if err != nil {
		fmt.Printf("⚠️ Falló switch a Go %s para coverage.\n", b.targetVersion)
	} else {
		defer b.switchGoVersion(baseGoVersion)
	}

	fmt.Println("--- Generando reporte de cobertura Go ---")

	env := append(os.Environ(), "GOTOOLCHAIN=local")

	cmd := exec.Command("go", "test", "-coverprofile=coverage.out", "./...")
	cmd.Dir = proj.Path
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error en go test: %w", err)
	}
	fmt.Println("✅ Coverage de Go generado.")
	return nil
}