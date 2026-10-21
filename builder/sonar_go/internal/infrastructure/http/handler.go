package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"heatmetro-go/internal/application/usecases"
	"heatmetro-go/internal/domain/ports"
	"heatmetro-go/internal/domain/project"
	"heatmetro-go/internal/infrastructure/builders"
	"heatmetro-go/internal/infrastructure/database"
	"heatmetro-go/internal/infrastructure/javaenv"
	"heatmetro-go/internal/infrastructure/strategies"
)

type commitDataReq struct {
	Sha     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

type Handler struct {
	processor *usecases.ProjectProcessor
	db        *database.SQLiteAdapter
	gitCli    ports.GitClient
	jdkMgr    *javaenv.Manager
}

func NewHandler(processor *usecases.ProjectProcessor, db *database.SQLiteAdapter, gitCli ports.GitClient, jdkMgr *javaenv.Manager) *Handler {
	return &Handler{
		processor: processor,
		db:        db,
		gitCli:    gitCli,
		jdkMgr:    jdkMgr,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/download-db", h.HandleDownloadDB)
	mux.HandleFunc("/import-db", h.HandleImportDB)
	mux.HandleFunc("/analyze", h.HandleAnalyze)
}

func (h *Handler) HandleDownloadDB(w http.ResponseWriter, r *http.Request) {
	// 1. Forzar a SQLite a escribir todos los datos recientes en el archivo .db
	h.db.Checkpoint()

	// 2. Buscar primero en el volumen de Docker, y luego en la raíz local
	dbPath := "/app/data/repositories.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		dbPath = "repositories.db" // Fallback local
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		http.Error(w, "Base de datos no encontrada.", http.StatusNotFound)
		return
	}

	fmt.Println("Descargando base de datos consolidada...")
	w.Header().Set("Content-Disposition", "attachment; filename=repositories.db")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, dbPath)
}

func (h *Handler) HandleImportDB(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		http.Error(w, "Error procesando formulario", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("databaseFile")
	if err != nil {
		http.Error(w, "No se subió ningún archivo.", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fmt.Printf("Intento de importar base de datos desde: %s\n", header.Filename)

	// 1. Definir el directorio base asegurando que apunte al VOLUMEN de Docker
	baseDir := "/app/data"
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		baseDir = "." // Fallback local
	}

	// Las rutas ahora viven en el mismo disco virtual
	tempPath := filepath.Join(baseDir, "temp_import.db")
	targetPath := filepath.Join(baseDir, "repositories.db")

	// 2. Crear el archivo temporal en el destino final
	tempFile, err := os.Create(tempPath)
	if err != nil {
		http.Error(w, "Error creando archivo temporal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	io.Copy(tempFile, file)
	tempFile.Close()

	// 3. Validar el esquema de SQLite
	tempDbAdapter, err := database.NewSQLiteAdapter(tempPath)
	if err != nil || !tempDbAdapter.ValidateDBFile() {
		os.Remove(tempPath)
		http.Error(w, "Archivo inválido o esquema incompatible.", http.StatusBadRequest)
		return
	}
	tempDbAdapter.Close()

	// 4. Reemplazo Seguro
	h.db.Close() // Cerramos la conexión actual
	
	os.Remove(targetPath)
	os.Remove(targetPath + "-wal")
	os.Remove(targetPath + "-shm")
	
	// Como tempPath y targetPath están en /app/data, os.Rename funcionará perfectamente
	err = os.Rename(tempPath, targetPath)
	if err != nil {
		// Si ocurre una catástrofe, reabrimos la BD para que el servidor no quede inutilizable
		h.db, _ = database.NewSQLiteAdapter(targetPath)
		http.Error(w, "Error reemplazando archivo original: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 5. Reiniciar la BD con el archivo importado
	h.db, _ = database.NewSQLiteAdapter(targetPath)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Base de datos importada y cargada correctamente."})
}

func (h *Handler) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	// Configuración Server-Sent Events (SSE)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming no soportado", http.StatusInternalServerError)
		return
	}

	// NUEVO: Agregamos el parámetro 'progress' (int) al evento
	sendEvent := func(status, message, step string, progress int) {
		event := map[string]interface{}{
			"status":   status,
			"message":  message,
			"step":     step,
			"progress": progress,
		}
		jsonEvent, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", jsonEvent)
		flusher.Flush()
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		sendEvent("error", "Falta el archivo sonar-project.properties", "", 0)
		return
	}

	repoPath := r.FormValue("repoPath")
	commitDataRaw := r.FormValue("commitData")
	
	var commitData commitDataReq
	json.Unmarshal([]byte(commitDataRaw), &commitData)

	// Procesar properties
	propsFile, _, err := r.FormFile("sonarProperties")
	if err != nil {
		sendEvent("error", "Falta el archivo sonar-project.properties", "", 0)
		return
	}
	defer propsFile.Close()

	baseClonesPath := os.Getenv("CLONES_PATH")
	if baseClonesPath == "" {
		baseClonesPath = "../gh"
	}
	fullRepoPath := filepath.Join(baseClonesPath, filepath.Base(repoPath))

	// fmt.Printf("\n--- Nueva Petición de Análisis ---\n")
	// fmt.Printf("Repo: %s\n", fullRepoPath)
	// fmt.Printf("Commit: %s\n", commitData.Sha)

	// 1. Verificación en Base de Datos
	exists, _ := h.db.CheckCommitExists(commitData.Sha)
	if exists {
		fmt.Println("Commit ya analizado previamente. Retornando resultado inmediato.")
		sendEvent("completed", "Recuperado de base de datos (Ya analizado).", "completed", 100)
		return
	}

	// 2. Checkout de Git
	sendEvent("running", "Preparando código...", "checkout", 5)
	err = h.gitCli.CheckoutRepo(fullRepoPath, commitData.Sha)
	if err != nil {
		sendEvent("error", err.Error(), "error", 0)
		return
	}

	// 3. Verificación de cambios en el código
	sendEvent("running", "Verificando cambios en código...", "validating", 10)
	hasChanges, _ := h.gitCli.HasCodeChanges(fullRepoPath, commitData.Sha)
	if !hasChanges {
		sendEvent("skipped", "Análisis omitido: No hay cambios en archivos de código.", "skipped", 100)
		return
	}

	// 4. Inyección de Estrategias y Compiladores según el lenguaje
	lang := project.DetectLanguage(fullRepoPath)
	fmt.Printf("Lenguaje detectado: %s\n", lang)

	// Detectar proyecto de Sonar del Properties
	propsTempPath := filepath.Join(fullRepoPath, "sonar-project.properties")
	destFile, _ := os.Create(propsTempPath)
	io.Copy(destFile, propsFile)
	destFile.Close()
	defer os.Remove(propsTempPath) // Limpieza del archivo temporal

	// Parseo simple para obtener exclusions (simulado del JS)
	projectKey, _ := h.parseSonarProperties(propsTempPath)
	exclusions := []string{} 

	if projectKey == "" {
		sendEvent("error", "No se encontró sonar.projectKey en el archivo sonar-project.properties", "", 0)
        return
    }
	
	proj := project.Project{
		Path:       fullRepoPath,
		ProjectKey: projectKey,
		Exclusions: exclusions,
		Language:   lang,
	}

	var strategy ports.AnalyzerStrategy
	var builder ports.BuildTool

	switch lang {
	case "GO":
		strategy = strategies.NewGoStrategy()
		builder = builders.NewGoBuilder()
	case "JAVA":
		strategy = strategies.NewJavaStrategy()
		builder = builders.NewMavenBuilder(h.jdkMgr) // Pasamos el JdkManager inyectado
	default:
		sendEvent("error", "Lenguaje no soportado", "error", 0)
		return
	}

	// 5. Análisis Principal Orquestado
	onProgress := func(percentage int, step, message string) {
		sendEvent("running", message, step, percentage)
	}

	err = h.processor.RunFullAnalysis(proj, strategy, builder, onProgress)
	if err != nil {
		sendEvent("error", err.Error(), "error", 0)
		return
	}

	sendEvent("completed", "Análisis finalizado y guardado.", "completed", 100)
}

func (h *Handler) parseSonarProperties(filePath string) (string, []string) {
    content, err := os.ReadFile(filePath)
    if err != nil {
        return "", nil
    }

    var projectKey string
    var exclusions []string
    lines := strings.Split(string(content), "\n")

    for _, line := range lines {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "sonar.projectKey=") {
            projectKey = strings.TrimSpace(strings.TrimPrefix(line, "sonar.projectKey="))
        } else if strings.HasPrefix(line, "sonar.exclusions=") {
            exclStr := strings.TrimSpace(strings.TrimPrefix(line, "sonar.exclusions="))
            parts := strings.Split(exclStr, ",")
            for _, p := range parts {
                clean := strings.TrimSpace(p)
                if clean != "" {
                    exclusions = append(exclusions, clean)
                }
            }
        }
    }
    return projectKey, exclusions
}