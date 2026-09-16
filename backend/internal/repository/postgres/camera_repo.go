package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
)

type CameraRepo struct {
	db *pgxpool.Pool
}

func NewCameraRepo(db *pgxpool.Pool) *CameraRepo {
	return &CameraRepo{db: db}
}

func (r *CameraRepo) List(ctx context.Context) ([]domain.Camera, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, rtsp_url,
			COALESCE(main_stream, '') as main_stream,
			COALESCE(sub_stream, '') as sub_stream,
			COALESCE(ip, '') as ip,
			COALESCE(mac, '') as mac,
			COALESCE(firmware, '') as firmware,
			site_id, COALESCE(wg_ip::text, '') as wg_ip, status, hw_info, settings, created_at, updated_at
		FROM cameras ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cameras := make([]domain.Camera, 0)
	for rows.Next() {
		var c domain.Camera
		var hwInfo, settings []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.RTSPUrl, &c.MainStream, &c.SubStream, &c.IP, &c.MAC, &c.Firmware,
			&c.SiteID, &c.WGIP,
			&c.Status, &hwInfo, &settings, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if hwInfo != nil {
			json.Unmarshal(hwInfo, &c.HWInfo)
		}
		if settings != nil {
			json.Unmarshal(settings, &c.Settings)
		}
		applySettings(&c)
		cameras = append(cameras, c)
	}
	return cameras, nil
}

// applySettings переносит значения из JSONB-поля settings в поля структуры.
// Отдельные колонки под них не заводим: набор настроек будет расширяться,
// а схему БД менять при каждом новом флаге неудобно.
func applySettings(c *domain.Camera) {
	if c.Settings == nil {
		return
	}
	if v, ok := c.Settings["ptz"].(bool); ok {
		c.PTZ = v
	}
}

func (r *CameraRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Camera, error) {
	var c domain.Camera
	var hwInfo, settings []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, name, rtsp_url,
			COALESCE(main_stream, '') as main_stream,
			COALESCE(sub_stream, '') as sub_stream,
			COALESCE(ip, '') as ip,
			COALESCE(mac, '') as mac,
			COALESCE(firmware, '') as firmware,
			site_id, COALESCE(wg_ip::text, '') as wg_ip, status, hw_info, settings, created_at, updated_at
		FROM cameras WHERE id = $1
	`, id).Scan(&c.ID, &c.Name, &c.RTSPUrl, &c.MainStream, &c.SubStream, &c.IP, &c.MAC, &c.Firmware,
		&c.SiteID, &c.WGIP,
		&c.Status, &hwInfo, &settings, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if hwInfo != nil {
		json.Unmarshal(hwInfo, &c.HWInfo)
	}
	if settings != nil {
		json.Unmarshal(settings, &c.Settings)
	}
	applySettings(&c)
	return &c, nil
}

func (r *CameraRepo) Create(ctx context.Context, cam *domain.Camera) error {
	hwInfo, _ := json.Marshal(cam.HWInfo)
	settings, _ := json.Marshal(cam.Settings)
	_, err := r.db.Exec(ctx, `
		INSERT INTO cameras (id, name, rtsp_url, main_stream, sub_stream, ip, mac, firmware, site_id, wg_ip, status, hw_info, settings, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, '')::inet, $11, $12, $13, $14, $15)
	`, cam.ID, cam.Name, cam.RTSPUrl, cam.MainStream, cam.SubStream, cam.IP, cam.MAC, cam.Firmware,
		cam.SiteID, cam.WGIP,
		cam.Status, hwInfo, settings, cam.CreatedAt, cam.UpdatedAt)
	return err
}

func (r *CameraRepo) Update(ctx context.Context, cam *domain.Camera) error {
	hwInfo, _ := json.Marshal(cam.HWInfo)
	settings, _ := json.Marshal(cam.Settings)
	cam.UpdatedAt = time.Now()
	_, err := r.db.Exec(ctx, `
		UPDATE cameras SET name=$2, rtsp_url=$3, main_stream=$4, sub_stream=$5, ip=$6, mac=$7, firmware=$8,
		site_id=$9, wg_ip=NULLIF($10, '')::inet, status=$11,
		hw_info=$12, settings=$13, updated_at=$14 WHERE id=$1
	`, cam.ID, cam.Name, cam.RTSPUrl, cam.MainStream, cam.SubStream, cam.IP, cam.MAC, cam.Firmware,
		cam.SiteID, cam.WGIP,
		cam.Status, hwInfo, settings, cam.UpdatedAt)
	return err
}

func (r *CameraRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM cameras WHERE id = $1`, id)
	return err
}

// UpdateStatus обновляет только поле status (используется монитором доступности).
func (r *CameraRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE cameras SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

// ListForStatusCheck возвращает минимальный набор полей для проверки доступности.
func (r *CameraRepo) ListForStatusCheck(ctx context.Context) ([]domain.Camera, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, COALESCE(ip, '') as ip,
			COALESCE(main_stream, '') as main_stream,
			COALESCE(sub_stream, '') as sub_stream,
			status
		FROM cameras
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cameras := make([]domain.Camera, 0)
	for rows.Next() {
		var c domain.Camera
		if err := rows.Scan(&c.ID, &c.Name, &c.IP, &c.MainStream, &c.SubStream, &c.Status); err != nil {
			return nil, err
		}
		cameras = append(cameras, c)
	}
	return cameras, rows.Err()
}
