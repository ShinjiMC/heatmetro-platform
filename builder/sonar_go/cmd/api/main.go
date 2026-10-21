package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"heatmetro-go/internal/application/usecases"
	"heatmetro-go/internal/domain/criticality"
	"heatmetro-go/internal/infrastructure/database"
	"heatmetro-go/internal/infrastructure/git"
	api_http "heatmetro-go/internal/infrastructure/http" // Corregido: Alias y ruta correcta
	"heatmetro-go/internal/infrastructure/javaenv"
	"heatmetro-go/internal/infrastructure/sonar"
)

func main() {
	fmt.Println("🔥 Iniciando Heatmetro Go API...")

	// 1. Cargar archivo .env (Igual que require('dotenv').config() en Node)
	err := godotenv.Load()
	if err != nil {
		fmt.Println("ℹ️ No se encontró archivo .env local, usando variables de entorno del sistema.")
	} else {
		fmt.Println("✅ Archivo .env cargado correctamente.")
	}

	// 2. Inicializar Base de Datos (Reemplaza initDb())
	// dbPath := "repositories.db"
	dbAdapter, err := database.NewSQLiteAdapter("/app/data/repositories.db")
	if err != nil {
		log.Fatalf("❌ Error crítico inicializando BD: %v\n", err)
	}
	defer dbAdapter.Close()

	// 2. Cargar Configuración
	sonarToken := os.Getenv("SONAR_TOKEN")
	if sonarToken == "" {
		fmt.Println("⚠️ ADVERTENCIA: SONAR_TOKEN no está definido en el entorno.")
	}

	// 3. Inicializar Adaptadores (Infraestructura)
	gitClient := git.NewCliAdapter()
	sonarClient := sonar.NewSonarClient(sonarToken)
	
	// Tu nuevo superpoder para manejar versiones antiguas de Java en empresas
	customJavaPaths := map[string]string{
		"4": "/opt/jdks/java4",
		"8": "/opt/jdks/java8_custom",
	}
	jdkManager := javaenv.NewManager(customJavaPaths)

	// 4. Inicializar Lógica de Negocio (Dominio)
	criticalityService := criticality.NewService()

	// 5. Inicializar Casos de Uso (Aplicación)
	projectProcessor := usecases.NewProjectProcessor(dbAdapter, gitClient, sonarClient, criticalityService)

	// 6. Configurar Servidor HTTP y Enrutar
	mux := http.NewServeMux()
	handler := api_http.NewHandler(projectProcessor, dbAdapter, gitClient, jdkManager) // Corregido: Uso del nuevo alias
	handler.RegisterRoutes(mux)

	// 7. Middleware CORS
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
			if r.Method == "OPTIONS" {
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	port := ":3001"
	fmt.Printf("🚀 Servidor de análisis corriendo en http://localhost%s\n", port)
	if err := http.ListenAndServe(port, corsMiddleware(mux)); err != nil {
		log.Fatalf("❌ Error del servidor: %v", err)
	}
}