package criticality

import (
	"heatmetro-go/internal/domain/metrics"
)

// Severity define el tipo estricto para los niveles de severidad
type Severity string

const (
	Low      Severity = "LOW"
	Medium   Severity = "MEDIUM"
	High     Severity = "HIGH"
	Critical Severity = "CRITICAL"
)

var severityWeight = map[Severity]int{
	Low:      1,
	Medium:   2,
	High:     3,
	Critical: 4,
}

// FileCriticality representa el resultado de la evaluación de un archivo
type FileCriticality struct {
	FilePath           string
	SeverityComplexity Severity // CC
	SeverityCoupling   Severity // CBO
	SeverityChurn      Severity // Churn
	SeverityAuthors    Severity // Gini (Fragmentación de autores)
	OverallSeverity    Severity // Representa la función booleana Crit(c)
}

// Service es el servicio de dominio que encapsula las reglas de negocio puras.
type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) CalculateFile(m *metrics.FileMetrics) FileCriticality {
	if m.Sonar.Coverage == -1 {
		return FileCriticality{
			FilePath:           m.FilePath,
			SeverityComplexity: Low,
			SeverityCoupling:   Low,
			SeverityChurn:      Low,
			SeverityAuthors:    Low,
			OverallSeverity:    Low,
		}
	}

	// Prevenir divisiones por cero obteniendo LOC reales o fallback a Churn/1
	loc := m.Cohesion.LOC
	if loc < 0 {
		print("Archivo con menor a 0 LOC: " + m.FilePath + "\n")
		return FileCriticality{
			FilePath:           m.FilePath,
			SeverityComplexity: Low,
			SeverityCoupling:   Low,
			SeverityChurn:      Low,
			SeverityAuthors:    Low,
			OverallSeverity:    Low,
		}
	}
	if loc == 0 {
		print("Archivo con 0 LOC: " + m.FilePath + "\n")
		loc = 1
	}


	crit := FileCriticality{
		FilePath: m.FilePath,
		
		// CC (Cyclomatic Complexity): Umbral absoluto de 10 caminos independientes
		SeverityComplexity: s.evaluateAbsolute(float64(m.Complexity), 10.0, 7.0, 4.0),
		
		// CBO (Coupling Between Objects): Umbral absoluto de 9 dependencias
		SeverityCoupling:   s.evaluateAbsolute(float64(m.Coupling.NumDependency), 9.0, 6.0, 3.0),
		
		// Churn (Volatilidad Evolutiva): Ratio > 20% (0.20) sobre el tamaño del archivo
		SeverityChurn:      s.evaluateRatio(float64(m.Churn.Frequency), loc, 0.20, 0.12, 0.05),
		
		// Authors (Fragmentación de Autores - Gini): Umbral absoluto del coeficiente > 0.75
		SeverityAuthors:    s.evaluateAbsolute(float64(m.Churn.Authors), 0.75, 0.50, 0.25),
	}

	crit.OverallSeverity = s.getHighestSeverity([]Severity{
		crit.SeverityComplexity,
		crit.SeverityCoupling,
		crit.SeverityChurn,
		crit.SeverityAuthors,
	})

	return crit
}

func (s *Service) evaluateRatio(value float64, loc int, critical, high, medium float64) Severity {
	ratio := value / float64(loc)

	if ratio >= critical {
		return Critical
	}
	if ratio >= high {
		return High
	}
	if ratio >= medium {
		return Medium
	}
	return Low
}

// evaluateAbsolute evalúa métricas absolutas.
func (s *Service) evaluateAbsolute(value, critical, high, medium float64) Severity {
	if value >= critical {
		return Critical
	}
	if value >= high {
		return High
	}
	if value >= medium {
		return Medium
	}
	return Low
}

// getHighestSeverity obtiene el nivel más alto de un conjunto de severidades.
func (s *Service) getHighestSeverity(severities []Severity) Severity {
	max := Low
	for _, sev := range severities {
		if severityWeight[sev] > severityWeight[max] {
			max = sev
		}
	}
	return max
}