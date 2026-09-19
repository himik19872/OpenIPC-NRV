package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
)

// RetentionRepo — доступ к архиву для сервиса очистки.
type RetentionRepo struct {
	db *pgxpool.Pool
}

func NewRetentionRepo(db *pgxpool.Pool) *RetentionRepo {
	return &RetentionRepo{db: db}
}

// ExpiredRecordings возвращает завершённые записи старше cutoff.
func (r *RetentionRepo) ExpiredRecordings(ctx context.Context, cutoff time.Time) ([]domain.ExpiredItem, error) {
	const q = `
		SELECT id::text, COALESCE(file_path, '')
		FROM recordings
		WHERE end_time < $1 AND file_path IS NOT NULL AND file_path <> ''`

	rows, err := r.db.Query(ctx, q, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query expired recordings: %w", err)
	}
	defer rows.Close()

	var out []domain.ExpiredItem
	for rows.Next() {
		var it domain.ExpiredItem
		if err := rows.Scan(&it.ID, &it.Path); err != nil {
			return nil, fmt.Errorf("scan expired recording: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ExpiredEvents возвращает события со снимками старше cutoff.
func (r *RetentionRepo) ExpiredEvents(ctx context.Context, cutoff time.Time) ([]domain.ExpiredItem, error) {
	const q = `
		SELECT id::text, COALESCE(snapshot_path, '')
		FROM detection_events
		WHERE timestamp < $1 AND snapshot_path IS NOT NULL AND snapshot_path <> ''`

	rows, err := r.db.Query(ctx, q, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query expired snapshots: %w", err)
	}
	defer rows.Close()

	var out []domain.ExpiredItem
	for rows.Next() {
		var it domain.ExpiredItem
		if err := rows.Scan(&it.ID, &it.Path); err != nil {
			return nil, fmt.Errorf("scan expired snapshot: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// DeleteRecordings удаляет записи архива по идентификаторам.
func (r *RetentionRepo) DeleteRecordings(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `DELETE FROM recordings WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return fmt.Errorf("delete recordings: %w", err)
	}
	return nil
}

// ClearEventSnapshots обнуляет ссылки на снимки, оставляя сами события.
// События нужны для статистики, поэтому их не удаляем — только картинку.
func (r *RetentionRepo) ClearEventSnapshots(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx,
		`UPDATE detection_events SET snapshot_path = NULL WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return fmt.Errorf("clear event snapshots: %w", err)
	}
	return nil
}

// StorageUsage возвращает текущий объём архива по данным БД.
func (r *RetentionRepo) StorageUsage(ctx context.Context) (recordingsBytes, snapshotsCount int64, err error) {
	row := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(file_size), 0) FROM recordings`)
	if err = row.Scan(&recordingsBytes); err != nil {
		return 0, 0, fmt.Errorf("sum recordings: %w", err)
	}

	row = r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM detection_events WHERE snapshot_path IS NOT NULL`)
	if err = row.Scan(&snapshotsCount); err != nil {
		return 0, 0, fmt.Errorf("count snapshots: %w", err)
	}
	return recordingsBytes, snapshotsCount, nil
}
