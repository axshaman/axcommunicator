package db

import (
    "database/sql"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "time"

    _ "github.com/mattn/go-sqlite3"
    "go.uber.org/zap"
)

var db *sql.DB

// InitDB initializes the SQLite database and returns a connection
func InitDB(logger *zap.Logger) (*sql.DB, error) {
    dbPath := os.Getenv("DB_PATH")
    if dbPath == "" {
        dbPath = "/app/database/comms.db"
    }

    if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
        return nil, fmt.Errorf("failed to create database directory: %w", err)
    }

    var err error
    db, err = sql.Open("sqlite3", dbPath)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }

    if err := db.Ping(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }

    if err := applyMigrations(logger); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to apply migrations: %w", err)
    }

    logger.Info("Database initialized", zap.String("path", dbPath))
    return db, nil
}

// applyMigrations applies SQL migration files from /app/migrations
func applyMigrations(logger *zap.Logger) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS migrations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            applied_at DATETIME NOT NULL
        )
    `)
    if err != nil {
        return fmt.Errorf("failed to create migrations table: %w", err)
    }

    migrationsDir := "/app/migrations"
    entries, err := os.ReadDir(migrationsDir)
    if err != nil {
        return fmt.Errorf("failed to read migrations directory: %w", err)
    }

    var migrationFiles []string
    for _, entry := range entries {
        if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
            migrationFiles = append(migrationFiles, entry.Name())
        }
    }
    sort.Strings(migrationFiles)

    tx, err := db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    applied := make(map[string]bool)
    rows, err := tx.Query("SELECT name FROM migrations")
    if err != nil {
        return fmt.Errorf("failed to query applied migrations: %w", err)
    }
    defer rows.Close()
    for rows.Next() {
        var name string
        if err := rows.Scan(&name); err != nil {
            return fmt.Errorf("failed to scan migration name: %w", err)
        }
        applied[name] = true
    }
    if err := rows.Err(); err != nil {
        return fmt.Errorf("error reading migrations: %w", err)
    }

    var appliedCount int
    var skippedCount int

    for _, file := range migrationFiles {
        if applied[file] {
            skippedCount++
            continue
        }

        sqlBytes, err := os.ReadFile(filepath.Join(migrationsDir, file))
        if err != nil {
            return fmt.Errorf("failed to read migration %s: %w", file, err)
        }

        if _, err := tx.Exec(string(sqlBytes)); err != nil {
            return fmt.Errorf("failed to apply migration %s: %w", file, err)
        }

        _, err = tx.Exec(
            "INSERT INTO migrations (name, applied_at) VALUES (?, ?)",
            file, time.Now().UTC(),
        )
        if err != nil {
            return fmt.Errorf("failed to record migration %s: %w", file, err)
        }

        logger.Info("Applied migration", zap.String("file", file))
        appliedCount++
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit migrations: %w", err)
    }

    if appliedCount == 0 {
        logger.Info("All migrations up to date", zap.Int("skipped", skippedCount))
    } else {
        logger.Info("Migration completed",
            zap.Int("applied", appliedCount),
            zap.Int("skipped", skippedCount))
    }

    return nil
}

// GetDB returns the database connection
func GetDB() *sql.DB {
    return db
}
