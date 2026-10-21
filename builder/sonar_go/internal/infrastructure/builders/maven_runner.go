package builders

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"heatmetro-go/internal/domain/project"
	"heatmetro-go/internal/infrastructure/javaenv"
)

// MavenBuilder implementa la interfaz ports.BuildTool
type MavenBuilder struct {
	jdkMgr   *javaenv.Manager
	javaHome string // Guardará la ruta resuelta para este proyecto específico
}

func NewMavenBuilder(jdkMgr *javaenv.Manager) *MavenBuilder {
	return &MavenBuilder{
		jdkMgr: jdkMgr,
	}
}

func (b *MavenBuilder) SetupEnvironment(proj project.Project) error {
	fmt.Printf("⚙️ Configurando entorno Java/Maven en: %s\n", proj.Path)
	pomFile := filepath.Join(proj.Path, "pom.xml")
	content, err := os.ReadFile(pomFile)
	version := "11"

	if err == nil {
		// Buscamos etiquetas estándar de versión en Maven
		re := regexp.MustCompile(`<java\.version>(.*?)<\/java\.version>|<maven\.compiler\.source>(.*?)<\/maven\.compiler\.source>`)
		match := re.FindStringSubmatch(string(content))
		if len(match) > 0 {
			if match[1] != "" {
				version = match[1]
			} else if match[2] != "" {
				version = match[2]
			}
		}
	}

	if version == "1.8" {
		version = "8"
	}

	fmt.Printf("   📌 Versión de Java detectada en pom.xml: %s\n", version)

	// 2. Obtener la ruta del JDK usando el Manager inyectado
	resolvedHome, err := b.jdkMgr.GetJavaHome(version)
	if err != nil {
		return fmt.Errorf("fallo al resolver entorno Java: %w", err)
	}
	
	b.javaHome = resolvedHome
	return nil
}

// injectJavaEnv sobreescribe JAVA_HOME y PATH exclusivamente para un comando
func (b *MavenBuilder) injectJavaEnv(cmd *exec.Cmd) {
	if b.javaHome == "" {
		return
	}

	// Construimos el nuevo PATH poniendo nuestro JDK de primero
	jdkBinPath := filepath.Join(b.javaHome, "bin")
	newPath := jdkBinPath + string(os.PathListSeparator) + os.Getenv("PATH")

	// Heredar entorno actual, pero omitiendo los viejos JAVA_HOME y PATH
	var customEnv []string
	for _, envVar := range os.Environ() {
		if !strings.HasPrefix(envVar, "JAVA_HOME=") && !strings.HasPrefix(envVar, "PATH=") {
			customEnv = append(customEnv, envVar)
		}
	}

	// Inyectar las variables nuevas
	customEnv = append(customEnv, "JAVA_HOME="+b.javaHome)
	customEnv = append(customEnv, "PATH="+newPath)

	cmd.Env = customEnv
}

func (b *MavenBuilder) Compile(proj project.Project) error {
	fmt.Println("   👉 Compilando proyecto Java (mvn clean compile)...")
	
	// DskipTests para compilar rápido sin correr pruebas aún
	cmd := exec.Command("mvn", "clean", "compile", "-DskipTests")
	cmd.Dir = proj.Path
	
	// ¡Aquí ocurre la magia de inyección!
	b.injectJavaEnv(cmd)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error compilando Java: %s\nSalida: %s", err, string(out))
	}
	return nil
}

func (b *MavenBuilder) GenerateCoverage(proj project.Project) error {
	fmt.Println("--- Generando reporte de cobertura Java (mvn test jacoco:report) ---")
	
	cmd := exec.Command("xvfb-run", "-a", "mvn", "test", "jacoco:report")
	// cmd := exec.Command("mvn", "test", "jacoco:report")
	cmd.Dir = proj.Path
	
	// Inyectamos el entorno para que las pruebas corran con la versión correcta
	b.injectJavaEnv(cmd)
	
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error en maven test: %w", err)
	}
	
	fmt.Println("✅ Coverage de Java generado.")
	return nil
}

// func (b *MavenBuilder) GenerateCoverage(proj project.Project) error {
// 	fmt.Println("   👉 2. Ejecutando Tests y JaCoCo...")
	
// 	// Preparamos agente JaCoCo
// 	jacocoVersion := "0.8.11"
// 	targetDir := filepath.Join(proj.Path, "target")
// 	os.MkdirAll(targetDir, os.ModePerm)

// 	// Descargar agente
// 	cmdAgent := exec.Command("mvn", "org.apache.maven.plugins:maven-dependency-plugin:3.6.0:copy",
// 		"-Dartifact=org.jacoco:org.jacoco.agent:"+jacocoVersion+":jar:runtime",
// 		"-DoutputDirectory="+targetDir)
// 	cmdAgent.Dir = proj.Path
// 	b.injectJavaEnv(cmdAgent, proj.Path)
// 	cmdAgent.Run()

// 	agentPath := filepath.Join(targetDir, fmt.Sprintf("org.jacoco.agent-%s-runtime.jar", jacocoVersion))
// 	destFile := filepath.Join(proj.Path, "target", "jacoco.exec")
// 	jacocoAgentArg := fmt.Sprintf("-javaagent:%s=destfile=%s -Djava.awt.headless=true", agentPath, destFile)

// 	// Correr Tests
// 	cmdTest := exec.Command("mvn", "test", "-Dmaven.test.failure.ignore=true", 
// 		"-DfailIfNoTests=false", "-Djacoco.skip=true", "-Dmaven.compiler.proc=none", 
// 		"-Drat.skip=true", "-Denforcer.skip=true", "-DargLine="+jacocoAgentArg, "-fn")
// 	cmdTest.Dir = proj.Path
// 	b.injectJavaEnv(cmdTest, proj.Path)
// 	cmdTest.Stdout = os.Stdout
// 	cmdTest.Stderr = os.Stderr
// 	cmdTest.Run()

// 	// Generar XML
// 	fmt.Println("   👉 3. Generando Reporte XML JaCoCo...")
// 	jacocoReport := fmt.Sprintf("org.jacoco:jacoco-maven-plugin:%s:report", jacocoVersion)
// 	cmdReport := exec.Command("mvn", jacocoReport, "-DfailIfNoTests=false", "-fn")
// 	cmdReport.Dir = proj.Path
// 	b.injectJavaEnv(cmdReport, proj.Path)
// 	cmdReport.Run()

// 	return nil
// }

// func (b *MavenBuilder) detectJavaVersion(projectPath string) string {
// 	pomPath := filepath.Join(projectPath, "pom.xml")
// 	content, err := os.ReadFile(pomPath)
// 	if err != nil {
// 		return "17" // Default
// 	}
	
// 	// Lógica legacy: Busca <java.version>1.8</java.version> o similar
// 	re := regexp.MustCompile(`<(?:java\.version|maven\.compiler\.source|source)>(.*?)<\/`)
// 	match := re.FindStringSubmatch(string(content))
// 	if len(match) > 1 {
// 		ver := strings.TrimSpace(match[1])
// 		if strings.HasPrefix(ver, "1.8") || ver == "8" || strings.HasPrefix(ver, "1.7") || ver == "7" {
// 			return "8"
// 		}
// 		return ver
// 	}
// 	return "17"
// }

// func (b *MavenBuilder) injectJavaEnv(cmd *exec.Cmd, projectPath string) {
// 	version := b.detectJavaVersion(projectPath)
// 	javaHome, _ := b.jdkManager.GetJavaHome(version) // Recuperamos la ruta local real

// 	// Inyectamos el JAVA_HOME modificado para que Maven compile con esta versión específica
// 	javaBin := filepath.Join(javaHome, "bin")
// 	newPath := fmt.Sprintf("%s%c%s", javaBin, os.PathListSeparator, os.Getenv("PATH"))
	
// 	cmd.Env = append(os.Environ(),
// 		"JAVA_HOME="+javaHome,
// 		"PATH="+newPath,
// 	)
// }