package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
)

type EventService struct {
	repo *postgres.EventRepo
}

func NewEventService(repo *postgres.EventRepo) *EventService {
	return &EventService{repo: repo}
}

func (s *EventService) List(ctx context.Context, cameraID *uuid.UUID, objectClass string, page, pageSize int) ([]domain.DetectionEvent, int64, error) {
	return s.repo.List(ctx, cameraID, objectClass, page, pageSize)
}

func (s *EventService) Get(ctx context.Context, id uuid.UUID) (*domain.DetectionEvent, error) {
	return s.repo.GetByID(ctx, id)
}
