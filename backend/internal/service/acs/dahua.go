package acs

import (
	"context"
	"fmt"

	"github.com/nvr/backend/internal/domain"
)

// DahuaAdapter — адаптер для контроллеров Dahua по HTTP API
type DahuaAdapter struct {
	ctrl *domain.ACSController
}

func NewDahuaAdapter(ctrl *domain.ACSController) (Adapter, error) {
	return &DahuaAdapter{ctrl: ctrl}, nil
}

func (a *DahuaAdapter) Ping(ctx context.Context) error {
	return fmt.Errorf("dahua adapter: not implemented yet")
}

func (a *DahuaAdapter) ListDoors(ctx context.Context) ([]Door, error) {
	return nil, fmt.Errorf("dahua adapter: not implemented yet")
}

func (a *DahuaAdapter) OpenDoor(ctx context.Context, doorID string) error {
	return fmt.Errorf("dahua adapter: not implemented yet")
}

func (a *DahuaAdapter) GetDoorStatus(ctx context.Context, doorID string) (DoorStatus, error) {
	return DoorStatus{}, fmt.Errorf("dahua adapter: not implemented yet")
}

func (a *DahuaAdapter) SubscribeEvents(ctx context.Context) (<-chan domain.ACSEvent, error) {
	return nil, fmt.Errorf("dahua adapter: not implemented yet")
}
