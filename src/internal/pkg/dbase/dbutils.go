package dbase

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/pressly/goose/v3"
	"log"
	"log/slog"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	DATABASE_PATH        = "/data/db/app.dbase"
	DATABASE_BACKUP_PATH = "/data/backups"
)

func InitDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", DATABASE_PATH)
	if err != nil {
		return nil, err
	}

	// Критически важные настройки для необслуживаемой БД
	pragmas := []string{
		"PRAGMA journal_mode=WAL",        // WAL-режим
		"PRAGMA synchronous=NORMAL",      // Баланс скорости и безопасности
		"PRAGMA auto_vacuum=INCREMENTAL", // Автоматическое освобождение места
		"PRAGMA busy_timeout=5000",       // Ждать при блокировке
		"PRAGMA foreign_keys=ON",         // Проверка FK
		"PRAGMA cache_size=-64000",       // 64 МБ кэш
		"PRAGMA temp_store=MEMORY",       // Временные данные в памяти
		"PRAGMA mmap_size=268435456",     // 256 МБ mmap для скорости
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("pragma failed: %w", err)
		}
	}

	// Применить миграции
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(2)

	return db, nil
}

func runMigrations(db *sql.DB) error {
	// Указать директорию с миграциями
	migrationsDir := "./migrations"

	// Применить все миграции
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}

	if err := goose.Up(db, migrationsDir); err != nil {
		return err
	}

	log.Println("Migrations applied successfully")
	return nil
}

func StartAutoMaintenance(ctx context.Context, logger *slog.Logger, db *sql.DB) {
	// 1. Ежечасное обслуживание (освобождение дырок + пассивный чекпоинт)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info("Hourly maintenance stopped")
				return
			case <-ticker.C:
				if _, err := db.Exec("PRAGMA incremental_vacuum(100)"); err != nil {
					logger.Error("Incremental vacuum failed", "error", err)
				}
				if _, err := db.Exec("PRAGMA wal_checkpoint(PASSIVE)"); err != nil {
					logger.Error("Passive checkpoint failed", "error", err)
				}
			}
		}
	}()

	// 2. Ежедневный полный чекпоинт (ночью)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info("Daily checkpoint stopped")
				return
			case <-ticker.C:
				if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
					logger.Error("Truncate checkpoint failed", "error", err)
				}
			}
		}
	}()

	// 3. Еженедельная проверка целостности
	go func() {
		ticker := time.NewTicker(7 * 24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info("Weekly integrity check stopped")
				return
			case <-ticker.C:
				var result string
				err := db.QueryRow("PRAGMA integrity_check").Scan(&result)
				if err != nil || result != "ok" {
					logger.Error("CRITICAL: Database corruption detected", "error", err, "result", result)
					// Здесь можно добавить отправку алерта (email, telegram и т.д.)
				}
			}
		}
	}()

	logger.Info("Auto maintenance tasks started")
}

func StartAutoBackup(ctx context.Context, logger *slog.Logger, db *sql.DB) {
	backupDir := DATABASE_BACKUP_PATH
	go func() {
		// 1. Обязательно останавливаем тикер при выходе из горутины
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			// 2. Слушаем сигнал отмены контекста
			case <-ctx.Done():
				logger.Info("Auto backup stopped gracefully")
				return // Выходим из горутины

			// 3. Слушаем срабатывание таймера
			case <-ticker.C:
				backupPath := filepath.Join(backupDir,
					fmt.Sprintf("backup_%s.dbase", time.Now().Format("2006-01-02")))

				if err := backupDB(db, backupPath, logger); err != nil {
					logger.Error("Backup failed", "error", err)
				} else {
					logger.Info("Backup created", "path", backupPath)

					// Удалить старые бэкапы (оставить последние 7)
					cleanupOldBackups(backupDir, 7)
				}
			}
		}
	}()
}

func backupDB(db *sql.DB, backupPath string, logger *slog.Logger) error {
	// Удалить старый файл, если существует (VACUUM INTO не перезаписывает)
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		logger.Error("failed to remove old backup", "error", err)
		return fmt.Errorf("failed to remove old backup: %w", err)
	}

	// Создать директорию, если не существует
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		logger.Error("failed to create backup directory", "error", err)
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Атомарный бэкап одной SQL-командой
	_, err := db.Exec(`VACUUM INTO ?`, backupPath)
	if err != nil {
		logger.Error("VACUUM INTO failed", "error", err)
		return fmt.Errorf("VACUUM INTO failed: %w", err)
	}

	return nil
}

func cleanupOldBackups(backupDir string, keepCount int) {
	files, _ := filepath.Glob(filepath.Join(backupDir, "backup_*.dbase"))
	sort.Strings(files)

	if len(files) > keepCount {
		for _, file := range files[:len(files)-keepCount] {
			os.Remove(file)
		}
	}
}

func ShutdownDB(db *sql.DB, logger *slog.Logger) error {
	logger.Info("Shutting down database...")

	// Установить таймаут на операции
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Отменить новые запросы (опционально)
	db.SetMaxOpenConns(0) // Запретить новые соединения

	// 2. Сделать checkpoint с контекстом
	done := make(chan error, 1)
	go func() {
		_, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			logger.Error("Warning: checkpoint failed", "error", err)
		}
	case <-ctx.Done():
		logger.Info("Checkpoint timeout, skipping")
	}

	// 3. Закрыть БД
	if err := db.Close(); err != nil {
		logger.Error("failed to close database", "error", err)
		return fmt.Errorf("failed to close database: %w", err)
	}

	logger.Info("Database closed successfully")
	return nil
}
