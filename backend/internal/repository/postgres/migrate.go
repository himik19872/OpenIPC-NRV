package postgres

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // драйвер database/sql поверх pgx
	"github.com/rs/zerolog/log"
)

// RunMigrations приводит схему БД к актуальной версии.
//
// Используется golang-migrate: он ведёт таблицу schema_migrations и
// применяет только недостающие версии, поэтому вызов безопасен при
// каждом старте сервера.
//
// migrationsPath — путь к каталогу с файлами вида `{version}_{name}.up.sql`
// в формате file:// (например, "file://migrations").
func RunMigrations(pool *pgxpool.Pool, migrationsPath string) error {
	// golang-migrate работает через database/sql, поэтому берём соединение
	// из того же пула pgx — так не появляется второй набор настроек.
	connStr := pool.Config().ConnString()
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("open sql connection: %w", err)
	}
	defer db.Close()

	driver, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(migrationsPath, "postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	// База могла быть создана до появления миграций (схема есть, а таблицы
	// schema_migrations нет). В этом случае помечаем первую версию применённой,
	// чтобы не пытаться создавать существующие таблицы заново.
	if err := adoptExistingSchema(m, db); err != nil {
		return err
	}

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info().Msg("database schema is up to date")
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil {
		log.Info().Msg("migrations applied")
		return nil
	}
	if dirty {
		return fmt.Errorf("database is in dirty state at version %d, manual fix required", version)
	}

	log.Info().Uint("version", version).Msg("migrations applied")
	return nil
}

// adoptExistingSchema помечает текущую версию применённой, если таблицы уже
// существуют, а записей о миграциях нет. Иначе migrate.Up() упадёт на
// "relation already exists".
func adoptExistingSchema(m *migrate.Migrate, db *sql.DB) error {
	if _, _, err := m.Version(); err == nil {
		// Таблица версий есть — обычный путь, ничего не делаем.
		return nil
	} else if !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("read schema version: %w", err)
	}

	var exists bool
	err := db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'cameras'
	)`).Scan(&exists)
	if err != nil || !exists {
		// База пустая — применяем миграции с нуля.
		return nil
	}

	// Схема уже есть. Версию, до которой она соответствует, определяем
	// по наличию колонок последней миграции.
	version := detectSchemaVersion(db)
	if version == 0 {
		return nil
	}

	log.Warn().Uint("version", version).
		Msg("existing schema detected without migration history, marking as applied")

	if err := m.Force(int(version)); err != nil {
		return fmt.Errorf("force version %d: %w", version, err)
	}
	return nil
}

// detectSchemaVersion определяет, какая миграция соответствует текущей схеме,
// по набору колонок. Возвращает 0, если определить не удалось.
func detectSchemaVersion(db *sql.DB) uint {
	var hasMainStream bool
	err := db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'cameras' AND column_name = 'main_stream'
	)`).Scan(&hasMainStream)
	if err != nil {
		return 0
	}
	// 002 добавляет поля двухпоточной модели, без них схема на версии 1.
	if hasMainStream {
		return 2
	}
	return 1
}
