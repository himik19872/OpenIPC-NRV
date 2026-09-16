package nats

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

type DetectionEvent struct {
	CameraID    string             `json:"camera_id"`
	Timestamp   float64            `json:"timestamp"`
	ObjectClass string             `json:"object_class"`
	Confidence  float64            `json:"confidence"`
	BBox        map[string]float64 `json:"bbox"`
	TrackID     *int               `json:"track_id"`
}

// DetectionSubscriber подписывается на NATS и сохраняет детекции в БД
type DetectionSubscriber struct {
	nc *nats.Conn
	db *pgxpool.Pool
}

func NewDetectionSubscriber(natsURL string, db *pgxpool.Pool) (*DetectionSubscriber, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}
	log.Info().Str("url", natsURL).Msg("connected to NATS for detection subscriber")
	return &DetectionSubscriber{nc: nc, db: db}, nil
}

func (s *DetectionSubscriber) Start(ctx context.Context) error {
	// Подписываемся на все детекции со всех камер
	_, err := s.nc.Subscribe("cameras.*.detection", func(msg *nats.Msg) {
		var ev DetectionEvent
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			log.Warn().Err(err).Str("subject", msg.Subject).Msg("failed to parse detection event")
			return
		}

		if err := s.saveEvent(ctx, &ev); err != nil {
			log.Error().Err(err).Str("camera_id", ev.CameraID).Msg("failed to save detection event")
		}
	})
	if err != nil {
		return err
	}

	log.Info().Msg("detection subscriber started (cameras.*.detection)")
	return nil
}

func (s *DetectionSubscriber) saveEvent(ctx context.Context, ev *DetectionEvent) error {
	eventID := uuid.New()
	eventTime := time.Unix(int64(ev.Timestamp), int64((ev.Timestamp-float64(int64(ev.Timestamp)))*1e9))

	cameraID, err := uuid.Parse(ev.CameraID)
	if err != nil {
		log.Warn().Str("raw_id", ev.CameraID).Msg("invalid camera UUID, skipping")
		return nil
	}

	bboxJSON, _ := json.Marshal(ev.BBox)

	_, err = s.db.Exec(ctx, `
		INSERT INTO detection_events (id, camera_id, timestamp, object_class, confidence, bbox, track_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, eventID, cameraID, eventTime, ev.ObjectClass, ev.Confidence, bboxJSON, ev.TrackID)

	if err != nil {
		return err
	}

	log.Debug().
		Str("camera_id", ev.CameraID[:8]).
		Str("class", ev.ObjectClass).
		Float64("conf", ev.Confidence).
		Msg("detection saved")

	return nil
}

func (s *DetectionSubscriber) Close() {
	if s.nc != nil {
		s.nc.Close()
	}
}
