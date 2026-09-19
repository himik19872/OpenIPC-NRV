package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/nvr/backend/internal/service/acs"
)

type ACSService struct {
	manager    *acs.Manager
	repo       *postgres.ACSRepo
	cameraRepo *postgres.CameraRepo
	eventRepo  *postgres.EventRepo
}

func NewACSService(manager *acs.Manager, cameraRepo *postgres.CameraRepo, eventRepo *postgres.EventRepo) *ACSService {
	return &ACSService{
		manager:    manager,
		repo:       manager.Repo(),
		cameraRepo: cameraRepo,
		eventRepo:  eventRepo,
	}
}

func (s *ACSService) ListControllers(ctx context.Context) ([]domain.ACSController, error) {
	return s.repo.List(ctx)
}

func (s *ACSService) GetController(ctx context.Context, id uuid.UUID) (*domain.ACSController, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ACSService) CreateController(ctx context.Context, req domain.CreateACSControllerRequest) (*domain.ACSController, error) {
	ctrl := &domain.ACSController{
		ID:        uuid.New(),
		Name:      req.Name,
		Vendor:    req.Vendor,
		IP:        req.IP,
		Port:      req.Port,
		Status:    "offline",
		CreatedAt: time.Now(),
		Credentials: map[string]any{
			"login":    req.Login,
			"password": req.Password,
		},
	}
	if req.SiteID != "" {
		siteID, err := uuid.Parse(req.SiteID)
		if err == nil {
			ctrl.SiteID = &siteID
		}
	}
	if err := s.repo.Create(ctx, ctrl); err != nil {
		return nil, err
	}
	return ctrl, nil
}

func (s *ACSService) DeleteController(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *ACSService) ListEvents(ctx context.Context, page, pageSize int) ([]domain.ACSEvent, int64, error) {
	return s.repo.ListEvents(ctx, page, pageSize)
}

// IngestEvent сохраняет событие, пришедшее от контроллера (push-канал).
// ControllerIP — IP контроллера, по которому он сопоставляется с записью в БД.
func (s *ACSService) IngestEvent(ctx context.Context, req domain.IngestACSEventRequest, controllerIP string) (*domain.ACSEvent, error) {
	ev := &domain.ACSEvent{
		ID:         uuid.New(),
		DoorID:     req.DoorID,
		EventType:  req.EventType,
		CardNumber: req.CardNumber,
		Metadata:   req.Metadata,
	}
	if ev.DoorID == "" {
		ev.DoorID = "door_1"
	}
	// Сопоставление события с контроллером по IP отправителя.
	if ctrl, err := s.repo.FindByIP(ctx, controllerIP); err == nil {
		ev.ControllerID = ctrl.ID
	}

	if ev.Metadata == nil {
		ev.Metadata = map[string]any{}
	}
	if req.Facility != 0 {
		ev.Metadata["facility"] = req.Facility
	}
	if req.Name != "" {
		ev.Metadata["name"] = req.Name
	}
	if req.Flags != 0 {
		ev.Metadata["flags"] = req.Flags
	}
	if req.DeviceID != "" {
		ev.Metadata["device_id"] = req.DeviceID
	}

	if req.Timestamp != 0 {
		ev.Timestamp = time.Unix(req.Timestamp, 0)
	} else {
		ev.Timestamp = time.Now()
	}

	if err := s.repo.SaveEvent(ctx, ev); err != nil {
		return nil, err
	}
	return ev, nil
}

func (s *ACSService) OpenDoor(ctx context.Context, controllerID uuid.UUID, doorID string) error {
	ctrl, err := s.repo.GetByID(ctx, controllerID)
	if err != nil {
		return fmt.Errorf("controller not found: %w", err)
	}

	adapter, err := s.manager.GetAdapter(ctrl.Vendor, ctrl)
	if err != nil {
		return fmt.Errorf("no adapter for vendor %s: %w", ctrl.Vendor, err)
	}

	return adapter.OpenDoor(ctx, doorID)
}
