package metrics

// FileMetrics consolida todas las métricas de un archivo individual.
// Esto reemplaza al inmenso "fileMap" que tenías en Node.js.
type FileMetrics struct {
	FilePath   string
	Type       string // FILE, PACKAGE, STRUCT
	Complexity int
	Churn      Churn
	Halstead   Halstead
	Coupling   Coupling
	Cohesion   Cohesion
	Sonar      SonarData
	Layout     Layout
}

// Churn representa la volatilidad del archivo.
type Churn struct {
	Added     int
	Deleted   int
	Total     int
	Frequency int
	Authors   int
}


// Halstead contiene las métricas de esfuerzo mental.
type Halstead struct {
	Volume     float64
	Difficulty float64
	Effort     float64
	Bugs       float64
}

// Coupling representa el Acoplamiento (Fan-in / Fan-out).
type Coupling struct {
	NumDependency int // Quién depende de mí (Fan-In)
	NumImports    int // De quién dependo (Fan-Out)
}

// Cohesion encapsula las líneas y métodos reales.
type Cohesion struct {
	Type        string
	LOC         int
	MethodCount int
	AttrCount   int
}

// SonarData almacena lo recolectado de SonarCloud.
type SonarData struct {
	Coverage     float64
	LinesToCover int
	Issues       int
	SonarURL     string
	IssuesURL    string
}

// Layout representa las coordenadas para la ciudad 3D.
type Layout struct {
	RootW  float64
	RootD  float64
	ChildW float64
	ChildD float64
	ChildX float64
	ChildY float64
}

// FolderMetrics almacena los acumulados Bottom-Up para las carpetas.
// Es la versión tipada de lo que generabas en hierarchy.js.
type FolderMetrics struct {
	FolderPath             string
	TotalIssues            int
	TotalComplexity        int
	TotalChurn             int
	TotalCouplingDeps      int
	TotalLOC               int
	TotalFuncCount         int
	TotalMethodCount       int
	TotalFrequency         int
	TotalAuthors           int
	TotalHalsteadVolume    float64
	TotalHalsteadDiff      float64
	TotalHalsteadEffort    float64
	TotalHalsteadBugs      float64
	AvgCoverage            float64
	DebtCovOverall         *float64 // Puntero para permitir NULL si no hay deuda
	DebtCovComplexity      *float64
	DebtCovCoupling        *float64
	DebtCovIssues          *float64
	DebtCovChurn           *float64
	DebtCovAuthors         *float64
	DebtCovHalstead        *float64
	SeverityComplexity     string
	SeverityCoupling       string
	SeverityIssues         string
	SeverityChurn          string
	SeverityAuthors        string
	SeverityHalstead       string
	OverallSeverity        string
	SonarURL               string
	IssuesURL              string
}