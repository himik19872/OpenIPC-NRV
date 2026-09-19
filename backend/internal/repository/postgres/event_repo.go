package postgres

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
	"github.com/rs/zerolog/log"
)

type EventRepo struct {
	db *pgxpool.Pool
}

func NewEventRepo(db *pgxpool.Pool) *EventRepo {
	return &EventRepo{db: db}
}

// List возвращает события с пагинацией. cameraID и objectClass —
// необязательные фильтры; пустые значения означают «без фильтра».
func (r *EventRepo) List(ctx context.Context, cameraID *uuid.UUID, objectClass string, page, pageSize int) ([]domain.DetectionEvent, int64, error) {
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if cameraID != nil {
		where += " AND camera_id = $" + itoa(argIdx)
		args = append(args, cameraID.String())
		argIdx++
	}

	// Фильтр по классу объекта: нужен для просмотра «только номера» или
	// «только люди» — иначе нужный кадр теряется среди остальных.
	if objectClass != "" {
		where += " AND object_class = $" + itoa(argIdx)
		args = append(args, objectClass)
		argIdx++
	}

	// Total count
	var total int64
	countQuery := "SELECT COUNT(*) FROM detection_events " + where
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	// snapshot_path и thumbnail_path могут быть NULL, а в domain это string:
	// без COALESCE rows.Scan падает, и запись молча теряется (было видно
	// «total есть, а список пуст»).
	query := `SELECT e.id, e.camera_id, e.timestamp, e.object_class, e.confidence,
		e.bbox, e.track_id, COALESCE(e.snapshot_path,''), COALESCE(e.thumbnail_path,''),
		COALESCE(e.metadata,'{}'),
		COALESCE(c.name, '') as camera_name,
		COALESCE(e.match_type,'unknown'), e.matched_id, COALESCE(e.matched_name,'')
		FROM detection_events e
		LEFT JOIN cameras c ON c.id = e.camera_id ` + where +
		` ORDER BY e.timestamp DESC LIMIT $` + itoa(argIdx) + ` OFFSET $` + itoa(argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := make([]domain.DetectionEvent, 0)
	for rows.Next() {
		var ev domain.DetectionEvent
		var bbox, metadata []byte
		if err := rows.Scan(&ev.ID, &ev.CameraID, &ev.Timestamp, &ev.ObjectClass,
			&ev.Confidence, &bbox, &ev.TrackID, &ev.SnapshotPath, &ev.ThumbnailPath,
			&metadata, &ev.CameraName, &ev.MatchType, &ev.MatchedID, &ev.MatchedName); err != nil {
			// Ошибку не глотаем молча: без лога причина «пустого списка»
			// при ненулевом total неочевидна.
			log.Warn().Err(err).Msg("не удалось прочитать событие детекции")
			continue
		}
		if bbox != nil {
			json.Unmarshal(bbox, &ev.BBox)
		}
		if metadata != nil {
			json.Unmarshal(metadata, &ev.Metadata)
		}
		events = append(events, ev)
	}
	return events, total, nil
}

func (r *EventRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.DetectionEvent, error) {
	var ev domain.DetectionEvent
	var bbox, metadata []byte
	err := r.db.QueryRow(ctx, `
		SELECT e.id, e.camera_id, e.timestamp, e.object_class, e.confidence,
		e.bbox, e.track_id, COALESCE(e.snapshot_path,''), COALESCE(e.thumbnail_path,''),
		COALESCE(e.metadata,'{}'),
		COALESCE(c.name, '') as camera_name,
		COALESCE(e.match_type,'unknown'), e.matched_id, COALESCE(e.matched_name,'')
		FROM detection_events e
		LEFT JOIN cameras c ON c.id = e.camera_id
		WHERE e.id = $1
	`, id).Scan(&ev.ID, &ev.CameraID, &ev.Timestamp, &ev.ObjectClass,
		&ev.Confidence, &bbox, &ev.TrackID, &ev.SnapshotPath, &ev.ThumbnailPath,
		&metadata, &ev.CameraName, &ev.MatchType, &ev.MatchedID, &ev.MatchedName)
	if err != nil {
		return nil, err
	}
	if bbox != nil {
		json.Unmarshal(bbox, &ev.BBox)
	}
	if metadata != nil {
		json.Unmarshal(metadata, &ev.Metadata)
	}
	return &ev, nil
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
