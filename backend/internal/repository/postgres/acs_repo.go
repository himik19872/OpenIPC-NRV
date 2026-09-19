package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
)

type ACSRepo struct {
	db *pgxpool.Pool
}

func NewACSRepo(db *pgxpool.Pool) *ACSRepo {
	return &ACSRepo{db: db}
}

func (r *ACSRepo) List(ctx context.Context) ([]domain.ACSController, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, vendor, ip, port, site_id, status, config, created_at
		FROM acs_controllers ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	controllers := make([]domain.ACSController, 0)
	for rows.Next() {
		var c domain.ACSController
		var cfg []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.Vendor, &c.IP, &c.Port,
			&c.SiteID, &c.Status, &cfg, &c.CreatedAt); err != nil {
			continue
		}
		if cfg != nil {
			json.Unmarshal(cfg, &c.Config)
		}
		controllers = append(controllers, c)
	}
	return controllers, nil
}

func (r *ACSRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.ACSController, error) {
	var c domain.ACSController
	var cfg, creds []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, name, vendor, ip, port, credentials, site_id, status, config, created_at
		FROM acs_controllers WHERE id = $1
	`, id).Scan(&c.ID, &c.Name, &c.Vendor, &c.IP, &c.Port,
		&creds, &c.SiteID, &c.Status, &cfg, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		json.Unmarshal(cfg, &c.Config)
	}
	if creds != nil {
		json.Unmarshal(creds, &c.Credentials)
	}
	return &c, nil
}

// FindByIP возвращает первый контроллер с заданным IP (для ingest).
func (r *ACSRepo) FindByIP(ctx context.Context, ip string) (*domain.ACSController, error) {
	var c domain.ACSController
	var cfg, creds []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, name, vendor, ip, port, credentials, site_id, status, config, created_at
		FROM acs_controllers WHERE ip = $1 LIMIT 1
	`, ip).Scan(&c.ID, &c.Name, &c.Vendor, &c.IP, &c.Port,
		&creds, &c.SiteID, &c.Status, &cfg, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		json.Unmarshal(cfg, &c.Config)
	}
	if creds != nil {
		json.Unmarshal(creds, &c.Credentials)
	}
	return &c, nil
}

func (r *ACSRepo) Create(ctx context.Context, c *domain.ACSController) error {
	creds, _ := json.Marshal(c.Credentials)
	cfg, _ := json.Marshal(c.Config)
	_, err := r.db.Exec(ctx, `
		INSERT INTO acs_controllers (id, name, vendor, ip, port, credentials, site_id, status, config, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, c.ID, c.Name, c.Vendor, c.IP, c.Port, creds, c.SiteID, c.Status, cfg, c.CreatedAt)
	return err
}

func (r *ACSRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM acs_controllers WHERE id = $1`, id)
	return err
}

// SaveEvent сохраняет событие СКУД (пришло от контроллера через ingest/push).
func (r *ACSRepo) SaveEvent(ctx context.Context, e *domain.ACSEvent) error {
	meta, _ := json.Marshal(e.Metadata)
	_, err := r.db.Exec(ctx, `
		INSERT INTO acs_events
			(id, controller_id, door_id, event_type, card_number, user_id,
			 timestamp, camera_id, snapshot_path, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, e.ID, e.ControllerID, e.DoorID, e.EventType, e.CardNumber, e.UserID,
		e.Timestamp, e.CameraID, e.SnapshotPath, meta)
	return err
}

func (r *ACSRepo) ListEvents(ctx context.Context, page, pageSize int) ([]domain.ACSEvent, int64, error) {
	var total int64
	r.db.QueryRow(ctx, `SELECT COUNT(*) FROM acs_events`).Scan(&total)

	offset := (page - 1) * pageSize
	rows, err := r.db.Query(ctx, `
		SELECT id, controller_id, door_id, event_type, card_number, user_id,
		timestamp, camera_id, snapshot_path, metadata
		FROM acs_events ORDER BY timestamp DESC LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := make([]domain.ACSEvent, 0)
	for rows.Next() {
		var e domain.ACSEvent
		var meta []byte
		if err := rows.Scan(&e.ID, &e.ControllerID, &e.DoorID, &e.EventType,
			&e.CardNumber, &e.UserID, &e.Timestamp, &e.CameraID,
			&e.SnapshotPath, &meta); err != nil {
			continue
		}
		if meta != nil {
			json.Unmarshal(meta, &e.Metadata)
		}
		events = append(events, e)
	}
	return events, total, nil
}
