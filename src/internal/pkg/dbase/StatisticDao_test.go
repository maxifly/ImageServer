package dbase

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite"
)

var testMigrationsDir = filepath.Join("..", "..", "..", "migrations")

// newTestDAO создаёт временную SQLite-базу,
// поднимает миграции goose и возвращает DAO.
func newTestDAO(tb testing.TB) *StatisticDao {
	tb.Helper()

	db := openTestDB(tb)
	runGooseMigrations(tb, db)

	return NewStatisticDao(db)
}

func openTestDB(tb testing.TB) *sql.DB {
	tb.Helper()

	dbPath := filepath.Join(tb.TempDir(), "test.db")

	// Для modernc.org/sqlite.
	// Если используешь mattn/go-sqlite3, см. примечание ниже.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)",
		dbPath,
	)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		tb.Fatalf("open test sqlite db: %v", err)
	}

	// Для SQLite в тестах обычно безопаснее иметь одно соединение.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		tb.Fatalf("ping test sqlite db: %v", err)
	}

	tb.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func runGooseMigrations(tb testing.TB, db *sql.DB) {
	tb.Helper()

	if err := goose.SetDialect("sqlite3"); err != nil {
		tb.Fatalf("set goose dialect: %v", err)
	}

	if err := goose.Up(db, testMigrationsDir); err != nil {
		tb.Fatalf("run goose migrations from %q: %v", testMigrationsDir, err)
	}
}

func TestStatisticDao_Increment(t *testing.T) {
	dao := newTestDAO(t)
	ctx := context.Background()

	code := "a"

	got, err := dao.Increment(ctx, code)
	if err != nil {
		t.Fatalf("first increment: %v", err)
	}

	if got != 1 {
		t.Fatalf("expected 1 after first increment, got %d", got)
	}

	got, err = dao.Increment(ctx, code)
	if err != nil {
		t.Fatalf("second increment: %v", err)
	}

	if got != 2 {
		t.Fatalf("expected 2 after second increment, got %d", got)
	}

	got, err = dao.Increment(ctx, code)
	if err != nil {
		t.Fatalf("third increment: %v", err)
	}

	if got != 3 {
		t.Fatalf("expected 3 after third increment, got %d", got)
	}

	// Другой код должен начинаться с 1.
	got, err = dao.Increment(ctx, "b")
	if err != nil {
		t.Fatalf("increment another code: %v", err)
	}

	if got != 1 {
		t.Fatalf("expected 1 for another code, got %d", got)
	}
}

func TestStatisticDao_Delete(t *testing.T) {
	dao := newTestDAO(t)
	ctx := context.Background()

	code := "a"

	_, err := dao.Increment(ctx, code)
	if err != nil {
		t.Fatalf("increment before delete: %v", err)
	}

	err = dao.Delete(ctx, code)
	if err != nil {
		t.Fatalf("delete existing code: %v", err)
	}

	all, err := dao.GetAll(ctx)
	if err != nil {
		t.Fatalf("get all after delete: %v", err)
	}

	if len(all) != 0 {
		t.Fatalf("expected empty map after delete, got %#v", all)
	}

	if err := dao.Delete(ctx, "missing"); err != nil {
		t.Fatalf("delete missing code should not fail: %v", err)
	}
}

func TestStatisticDao_GetAll(t *testing.T) {
	dao := newTestDAO(t)
	ctx := context.Background()

	// Сначала таблица пустая.
	all, err := dao.GetAll(ctx)
	if err != nil {
		t.Fatalf("get all empty: %v", err)
	}

	if len(all) != 0 {
		t.Fatalf("expected empty map, got %#v", all)
	}

	// Накручиваем счётчики.
	increments := map[string]int{
		"a": 3,
		"b": 1,
		"c": 5,
	}

	for code, count := range increments {
		for i := 0; i < count; i++ {
			_, err := dao.Increment(ctx, code)
			if err != nil {
				t.Fatalf("increment code %q: %v", code, err)
			}
		}
	}

	all, err = dao.GetAll(ctx)
	if err != nil {
		t.Fatalf("get all after increments: %v", err)
	}

	if len(all) != len(increments) {
		t.Fatalf("expected %d items, got %d: %#v", len(increments), len(all), all)
	}

	for code, count := range increments {
		requireMapCounter(t, all, code, int64(count))
	}
}

func TestStatisticDao_Increment_Concurrent(t *testing.T) {
	dao := newTestDAO(t)
	ctx := context.Background()

	const (
		code       = "concurrent"
		iterations = 25
	)

	var wg sync.WaitGroup
	errCh := make(chan error, iterations)

	for i := 0; i < iterations; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := dao.Increment(ctx, code)
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent increment error: %v", err)
	}

	all, err := dao.GetAll(ctx)
	if err != nil {
		t.Fatalf("get all after concurrent increments: %v", err)
	}

	requireMapCounter(t, all, code, iterations)
}

func requireMapCounter(
	tb testing.TB,
	m map[string]StatisticItem,
	code string,
	want int64,
) {
	tb.Helper()

	item, ok := m[code]
	if !ok {
		tb.Fatalf("expected code %q to exist in map %#v", code, m)
	}

	if item.UseCnt != want {
		tb.Fatalf("expected counter %d for code %q, got %d", want, code, item.UseCnt)
	}
}
