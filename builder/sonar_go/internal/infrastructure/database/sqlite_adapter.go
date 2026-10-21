package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"heatmetro-go/internal/domain/criticality"
	"heatmetro-go/internal/domain/metrics"

	// Usamos modernc/sqlite para no depender de CGO (node-gyp equivalent)
	_ "modernc.org/sqlite"
)

// SQLiteAdapter implementa ports.DBRepository
type SQLiteAdapter struct {
	db *sql.DB
}

// NewSQLiteAdapter inicializa la DB, configura PRAGMAS y crea el esquema
func NewSQLiteAdapter(dbPath string) (*SQLiteAdapter, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error abriendo DB: %w", err)
	}

	db.Exec("PRAGMA journal_mode = WAL;")
	db.Exec("PRAGMA foreign_keys = ON;")

	adapter := &SQLiteAdapter{db: db}

	err = adapter.setupSchema()
	if err != nil {
		return nil, err
	}

	return adapter, nil
}


func (a *SQLiteAdapter) setupSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tbl_sha (
		sha_id      INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_text    TEXT NOT NULL,
		commit_date TEXT,
		commit_message TEXT,
		author_name TEXT,
		UNIQUE(sha_text)
	);

	CREATE TABLE IF NOT EXISTS tbl_churn (
		churn_id    INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id      INTEGER NOT NULL,
		file_path   TEXT NOT NULL,
		added       INTEGER DEFAULT 0,
		deleted     INTEGER DEFAULT 0,
		total       INTEGER DEFAULT 0,
		frequency   INTEGER DEFAULT 0,
		authors     INTEGER DEFAULT 0,
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tbl_cohesion (
		cohesion_id   INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id        INTEGER NOT NULL,
		file_path     TEXT NOT NULL,
		loc           INTEGER DEFAULT 0,
		func_count    INTEGER DEFAULT 0,
		method_count  INTEGER DEFAULT 0,
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tbl_complexity (
		complexity_id INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id        INTEGER NOT NULL,
		file_path     TEXT NOT NULL,
		value         INTEGER DEFAULT 0,
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tbl_coupling (
		coupling_id     INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id          INTEGER NOT NULL,
		file_path       TEXT NOT NULL,
		num_dependency  INTEGER DEFAULT 0,
		num_imports     INTEGER DEFAULT 0,
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tbl_coverage_summary (
		coverage_id INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id      INTEGER NOT NULL,
		file_path   TEXT NOT NULL,
		percentage  REAL DEFAULT 0,
		sonar_url   TEXT, 
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tbl_lint_summary (
		lint_id     INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id      INTEGER NOT NULL,
		file_path   TEXT NOT NULL,
		num_issues  INTEGER DEFAULT 0,
		issues_url  TEXT,
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tbl_file_criticality (
		file_crit_id        INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id              INTEGER NOT NULL,
		file_path           TEXT NOT NULL,
		severity_complexity TEXT DEFAULT 'LOW',
		severity_coupling   TEXT DEFAULT 'LOW',
		severity_churn      TEXT DEFAULT 'LOW',
		severity_authors    TEXT DEFAULT 'LOW',
		overall_severity    TEXT DEFAULT 'LOW', 
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE,
		UNIQUE(sha_id, file_path)
	);

	CREATE TABLE IF NOT EXISTS tbl_halstead (
		halstead_id         INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id              INTEGER NOT NULL,
		file_path           TEXT NOT NULL,
		volume              REAL,
		difficulty          REAL,
		effort              REAL,
		bugs                REAL,
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tbl_folder_metrics (
		folder_metric_id  INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id            INTEGER NOT NULL,
		folder_path       TEXT NOT NULL,
		total_issues      INTEGER DEFAULT 0,
		total_complexity  INTEGER DEFAULT 0,
		total_churn       INTEGER DEFAULT 0,
		total_coupling_deps INTEGER DEFAULT 0,
		total_loc         INTEGER DEFAULT 0,
		total_func_count  INTEGER DEFAULT 0,
		total_method_count INTEGER DEFAULT 0,
		avg_coverage      REAL DEFAULT 0,
		total_frequency   INTEGER DEFAULT 0,
		total_authors     INTEGER DEFAULT 0,
		total_halstead_volume    REAL DEFAULT 0,
		total_halstead_difficulty REAL DEFAULT 0,
		total_halstead_effort    REAL DEFAULT 0,
		total_halstead_bugs      REAL DEFAULT 0,
		severity_complexity TEXT DEFAULT 'LOW',
		severity_coupling   TEXT DEFAULT 'LOW',
		severity_churn      TEXT DEFAULT 'LOW',
		severity_authors    TEXT DEFAULT 'LOW',
		overall_severity    TEXT DEFAULT 'LOW',
		debt_cov_overall    REAL,
		debt_cov_complexity REAL,
		debt_cov_coupling   REAL,
		debt_cov_churn      REAL,
		debt_cov_authors    REAL,
		sonar_url           TEXT,
		issues_url          TEXT,
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE,
		UNIQUE(sha_id, folder_path)
	);

	CREATE TABLE IF NOT EXISTS tbl_folder_hierarchy (
		hierarchy_id  INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id        INTEGER NOT NULL,
		folder_path   TEXT NOT NULL,
		children      TEXT,
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE,
		UNIQUE(sha_id, folder_path)
	);

	CREATE TABLE IF NOT EXISTS tbl_city_layout (
		layout_id   INTEGER PRIMARY KEY AUTOINCREMENT,
		sha_id      INTEGER NOT NULL,
		path        TEXT NOT NULL,    
		type        TEXT NOT NULL,    
		root_w      REAL DEFAULT 0,
		root_d      REAL DEFAULT 0,
		child_w     REAL,            
		child_d     REAL,          
		child_x     REAL,            
		child_y     REAL,         
		FOREIGN KEY(sha_id) REFERENCES tbl_sha(sha_id) ON DELETE CASCADE,
		UNIQUE(sha_id, path)
	);`

	_, err := a.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("error generando esquema: %w", err)
	}
	return nil
}


func (a *SQLiteAdapter) CheckCommitExists(sha string) (bool, error) {
	var id int
	err := a.db.QueryRow("SELECT sha_id FROM tbl_sha WHERE sha_text = ?", sha).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a *SQLiteAdapter) SaveCommitInfo(sha, date, author, message string) (int64, error) {
	res, err := a.db.Exec(
		"INSERT INTO tbl_sha (sha_text, commit_date, author_name, commit_message) VALUES (?, ?, ?, ?)",
		sha, date, author, message,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SaveFileMetrics reemplaza tu largo archivo storage.service.js
func (a *SQLiteAdapter) SaveFileMetrics(shaID int64, files []metrics.FileMetrics) error {
	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}

	// Preparamos los Statements una sola vez para que Vuele (Alta concurrencia)
	stmtChurn, _ := tx.Prepare(`INSERT INTO tbl_churn (sha_id, file_path, added, deleted, total, frequency, authors) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	stmtHalstead, _ := tx.Prepare(`INSERT INTO tbl_halstead (sha_id, file_path, volume, difficulty, effort, bugs) VALUES (?, ?, ?, ?, ?, ?)`)
	stmtCoupling, _ := tx.Prepare(`INSERT INTO tbl_coupling (sha_id, file_path, num_dependency, num_imports) VALUES (?, ?, ?, ?)`)
	stmtCoverage, _ := tx.Prepare(`INSERT INTO tbl_coverage_summary (sha_id, file_path, percentage, sonar_url) VALUES (?, ?, ?, ?)`)
	stmtLint, _ := tx.Prepare(`INSERT INTO tbl_lint_summary (sha_id, file_path, num_issues, issues_url) VALUES (?, ?, ?, ?)`)
	stmtComplexity, _ := tx.Prepare(`INSERT INTO tbl_complexity (sha_id, file_path, value) VALUES (?, ?, ?)`)
	stmtCohesion, _ := tx.Prepare(`INSERT INTO tbl_cohesion (sha_id, file_path, loc, func_count, method_count) VALUES (?, ?, ?, ?, ?)`)
	stmtLayout, _ := tx.Prepare(`INSERT INTO tbl_city_layout (sha_id, path, type, root_w, root_d, child_w, child_d, child_x, child_y) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	// Limpiamos memoria al salir
	defer func() {
		stmtChurn.Close()
		stmtHalstead.Close()
		stmtCoupling.Close()
		stmtCoverage.Close()
		stmtLint.Close()
		stmtComplexity.Close()
		stmtCohesion.Close()
		stmtLayout.Close()
	}()

	for _, m := range files {
		isStruct := m.Type == "STRUCT"
		isPackage := m.Type == "PACKAGE"
		isFile := !isStruct && !isPackage

		// 1. Específicas de Archivos puros
		if isFile {
			_, err = stmtChurn.Exec(shaID, m.FilePath, m.Churn.Added, m.Churn.Deleted, m.Churn.Total, m.Churn.Frequency, m.Churn.Authors)
			_, err = stmtHalstead.Exec(shaID, m.FilePath, m.Halstead.Volume, m.Halstead.Difficulty, m.Halstead.Effort, m.Halstead.Bugs)
			_, err = stmtCoupling.Exec(shaID, m.FilePath, m.Coupling.NumDependency, m.Coupling.NumImports)
			_, err = stmtCoverage.Exec(shaID, m.FilePath, m.Sonar.Coverage, m.Sonar.SonarURL)
			_, err = stmtLint.Exec(shaID, m.FilePath, m.Sonar.Issues, m.Sonar.IssuesURL)
			_, err = stmtComplexity.Exec(shaID, m.FilePath, m.Complexity)
		}

		// 2. Cohesión y Layout se insertan para todos (Packages, Structs y Files)
		realLoc := m.Cohesion.LOC
		if realLoc == 0 && isFile {
			realLoc = m.Churn.Total
		}
		
		_, err = stmtCohesion.Exec(shaID, m.FilePath, realLoc, m.Cohesion.AttrCount, m.Cohesion.MethodCount)

		// Si tiene layout tipo definido
		if m.Layout.RootW > 0 || m.Type != "" {
			layoutType := m.Type
			if layoutType == "" {
				layoutType = "FILE"
			}
			_, err = stmtLayout.Exec(shaID, m.FilePath, layoutType, m.Layout.RootW, m.Layout.RootD, m.Layout.ChildW, m.Layout.ChildD, m.Layout.ChildX, m.Layout.ChildY)
		}

		// Validamos errores en la iteración actual
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error insertando archivo %s: %w", m.FilePath, err)
		}
	}

	return tx.Commit()
}


func (a *SQLiteAdapter) SaveFolderMetrics(shaID int64, folders map[string]metrics.FolderMetrics) error {
	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}

	stmtFolder, _ := tx.Prepare(`
		INSERT INTO tbl_folder_metrics (
			sha_id, folder_path, total_issues, total_complexity, total_churn, total_coupling_deps, 
			total_loc, total_func_count, total_method_count, total_frequency, total_authors, 
			total_halstead_volume, total_halstead_difficulty, total_halstead_effort, total_halstead_bugs, avg_coverage,
			severity_complexity, severity_coupling, severity_churn, severity_authors, overall_severity,
			debt_cov_overall, debt_cov_complexity, debt_cov_coupling, debt_cov_churn, debt_cov_authors,
			sonar_url, issues_url
		) VALUES (
			?, ?, ?, ?, ?, ?, 
			?, ?, ?, ?, ?, 
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?
		)
	`)

	defer stmtFolder.Close()

	for path, stats := range folders {
		// En Go, pasar nil a la BD inserta un "NULL". Si la URL está vacía, usamos nil.
		var sUrl, iUrl *string
		if stats.SonarURL != "" { sUrl = &stats.SonarURL }
		if stats.IssuesURL != "" { iUrl = &stats.IssuesURL }

		_, err = stmtFolder.Exec(
			shaID, path,
			stats.TotalIssues, stats.TotalComplexity, stats.TotalChurn, stats.TotalCouplingDeps,
			stats.TotalLOC, stats.TotalFuncCount, stats.TotalMethodCount,
			stats.TotalFrequency, stats.TotalAuthors,
			stats.TotalHalsteadVolume, stats.TotalHalsteadDiff, stats.TotalHalsteadEffort, stats.TotalHalsteadBugs,
			stats.AvgCoverage,
			// Pasamos las severidades heredadas
			stats.SeverityComplexity, stats.SeverityCoupling, 
			stats.SeverityChurn, stats.SeverityAuthors, stats.OverallSeverity,
			// Pasamos la deuda calculada (pueden ser nil)
			stats.DebtCovOverall, stats.DebtCovComplexity, stats.DebtCovCoupling, 
			stats.DebtCovChurn, stats.DebtCovAuthors,
			sUrl, iUrl,
		)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (a *SQLiteAdapter) SaveFolderHierarchy(shaID int64, hierarchy map[string][]string) error {
	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}

	stmt, _ := tx.Prepare(`INSERT INTO tbl_folder_hierarchy (sha_id, folder_path, children) VALUES (?, ?, ?)`)
	defer stmt.Close()

	for path, children := range hierarchy {
		childrenJSON, _ := json.Marshal(children)
		_, err = stmt.Exec(shaID, path, string(childrenJSON))
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (a *SQLiteAdapter) SaveCriticality(shaID int64, criticalities []interface{}) error {
	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}

	stmtCrit, _ := tx.Prepare(`
		INSERT INTO tbl_file_criticality (
			sha_id, file_path, severity_complexity, severity_coupling, 
			severity_churn, severity_authors, overall_severity
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(sha_id, file_path) DO UPDATE SET
			severity_complexity = excluded.severity_complexity,
			severity_coupling = excluded.severity_coupling,
			severity_churn = excluded.severity_churn,
			severity_authors = excluded.severity_authors,
			overall_severity = excluded.overall_severity
	`)
	
	defer stmtCrit.Close()

	for _, item := range criticalities {
		// Type Assertion (Casteo seguro) a la entidad del dominio
		crit, ok := item.(criticality.FileCriticality)
		if !ok {
			continue
		}
		
		_, err = stmtCrit.Exec(
			shaID, crit.FilePath,
			string(crit.SeverityComplexity), 
			string(crit.SeverityCoupling),
			string(crit.SeverityChurn), string(crit.SeverityAuthors),
			string(crit.OverallSeverity),
		)

		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}


// ValidateDBFile comprueba si el archivo local cumple con las tablas requeridas (Equivalente al JS)
func (a *SQLiteAdapter) ValidateDBFile() bool {
	requiredTables := []string{
		"tbl_sha", "tbl_churn", "tbl_cohesion", "tbl_complexity",
		"tbl_coupling", "tbl_coverage_summary", "tbl_lint_summary",
		"tbl_file_criticality", "tbl_halstead", "tbl_folder_metrics",
		"tbl_folder_hierarchy", "tbl_city_layout",
	}

	rows, err := a.db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return false
	}
	defer rows.Close()

	existingTables := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			existingTables[name] = true
		}
	}

	for _, table := range requiredTables {
		if !existingTables[table] {
			fmt.Printf("❌ Validación fallida. Falta tabla: %s\n", table)
			return false
		}
	}

	return true
}

func (a *SQLiteAdapter) Close() error {
	return a.db.Close()
}

func (a *SQLiteAdapter) Checkpoint() {
	_, err := a.db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	if err != nil {
		fmt.Printf("⚠️ Advertencia: No se pudo hacer checkpoint de WAL: %v\n", err)
	}
}