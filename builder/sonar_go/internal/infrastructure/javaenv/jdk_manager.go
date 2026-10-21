package javaenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager se encarga de resolver la ruta física correcta de un JDK
// basado en la versión solicitada (ej: "4", "8", "17").
type Manager struct {
	// customPaths permite inyectar un mapa desde un JSON de configuración.
	// Ej: "4" -> "/mnt/volumen_empresa/java1.4_custom"
	customPaths map[string]string
}

func NewManager(paths map[string]string) *Manager {
	if paths == nil {
		paths = make(map[string]string)
	}
	return &Manager{
		customPaths: paths,
	}
}

// GetJavaHome devuelve la ruta absoluta del JAVA_HOME requerido.
// Implementa el puerto ports.JdkManager (que es usado por maven_builder.go)
func (m *Manager) GetJavaHome(version string) (string, error) {
	fmt.Printf("🔍 Buscando entorno local para Java %s...\n", version)

	// 1. Regla de Oro: Mapeo manual (Configuración externa JSON/YAML)
	if p, ok := m.customPaths[version]; ok {
		if m.isValidJDK(p) {
			fmt.Printf("   ✅ JDK Customizado encontrado: %s\n", p)
			return p, nil
		}
		fmt.Printf("   ⚠️ Ruta custom %s inválida o no contiene bin/java.\n", p)
	}

	// 2. Regla Docker Dinámica: Variables de Entorno específicas
	// Permite que en Docker pongas ENV JAVA_HOME_4=/opt/java4
	envKey := fmt.Sprintf("JAVA_HOME_%s", strings.ReplaceAll(version, ".", "_"))
	if envPath := os.Getenv(envKey); envPath != "" {
		if m.isValidJDK(envPath) {
			fmt.Printf("   ✅ JDK %s encontrado vía %s: %s\n", version, envKey, envPath)
			return envPath, nil
		}
	}

	// 3. Regla de Convención: Carpetas BYO-JDK y Linux Standars
	commonPaths := []string{
		fmt.Sprintf("/opt/jdks/java%s", version),                   // Carpeta montada recomendada
		fmt.Sprintf("/usr/lib/jvm/java-%s-openjdk-amd64", version), // Ubuntu Standard
		fmt.Sprintf("/usr/lib/jvm/java-%s-openjdk", version),       // Alpine y modernos locales (Java 11, 21)
		fmt.Sprintf("/usr/lib/jvm/java-1.%s.0-openjdk", version),   // Locales legacy (Java 8)
		fmt.Sprintf("/opt/java/openjdk%s", version),                // Docker Official Standard
	}

	for _, p := range commonPaths {
		if m.isValidJDK(p) {
			fmt.Printf("   ✅ JDK %s encontrado por convención en: %s\n", version, p)
			return p, nil
		}
	}

	// 4. Regla de Supervivencia: Fallback al JAVA_HOME del sistema
	// Esto asume que el sistema principal tiene un Java que "podría" funcionar.
	systemJava := os.Getenv("JAVA_HOME")
	if systemJava != "" && m.isValidJDK(systemJava) {
		fmt.Printf("   ⚠️ PELIGRO: Usando JAVA_HOME global como fallback para Java %s. Podría haber errores de compilación.\n", version)
		return systemJava, nil
	}

	// Si llegamos aquí, el análisis no puede continuar de forma segura
	return "", fmt.Errorf(`❌ ERROR CRÍTICO: No se encontró Java %s. 
Por favor, asegúrate de cumplir UNA de estas condiciones:
1. Definir la variable de entorno %s=/ruta/a/java
2. Montar un volumen Docker en /opt/jdks/java%s`, version, envKey, version)
}

// isValidJDK comprueba si el directorio realmente contiene el ejecutable de Java.
func (m *Manager) isValidJDK(path string) bool {
	// Comprobamos la existencia de "bin/java" o "bin/java.exe" en Windows
	binPath := filepath.Join(path, "bin", "java")
	if _, err := os.Stat(binPath); err == nil {
		return true
	}
	binPathExe := filepath.Join(path, "bin", "java.exe")
	if _, err := os.Stat(binPathExe); err == nil {
		return true
	}
	return false
}