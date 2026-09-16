package acs

import (
	"context"
	"fmt"

	"github.com/nvr/backend/internal/domain"
)

// PromwadAdapter — адаптер для контроллеров Promwad
type PromwadAdapter struct {
	ctrl *domain.ACSController
}

func NewPromwadAdapter(ctrl *domain.ACSController) (Adapter, error) {
	return &PromwadAdapter{ctrl: ctrl}, nil
}

func (a *PromwadAdapter) Ping(ctx context.Context) error {
	return fmt.Errorf("promwad adapter: not implemented yet")
}

func (a *PromwadAdapter) ListDoors(ctx context.Context) ([]Door, error) {
	return nil, fmt.Errorf("promwad adapter: not implemented yet")
}

func (a *PromwadAdapter) OpenDoor(ctx context.Context, doorID string) error {
	return fmt.Errorf("promwad adapter: not implemented yet")
}

func (a *PromwadAdapter) GetDoorStatus(ctx context.Context, doorID string) (DoorStatus, error) {
	return DoorStatus{}, fmt.Errorf("promwad adapter: not implemented yet")
}

func (a *PromwadAdapter) SubscribeEvents(ctx context.Context) (<-chan domain.ACSEvent, error) {
	return nil, fmt.Errorf("promwad adapter: not implemented yet")
}
