package acs

import (
	"context"
	"fmt"

	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
)

// Adapter — интерфейс адаптера СКУД
type Adapter interface {
	Ping(ctx context.Context) error
	ListDoors(ctx context.Context) ([]Door, error)
	OpenDoor(ctx context.Context, doorID string) error
	GetDoorStatus(ctx context.Context, doorID string) (DoorStatus, error)
	SubscribeEvents(ctx context.Context) (<-chan domain.ACSEvent, error)
}

type Door struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // locked, unlocked, open
}

type DoorStatus struct {
	Locked bool `json:"locked"`
	Open   bool `json:"open"`
	Alarm  bool `json:"alarm"`
}

// Manager управляет адаптерами СКУД
type Manager struct {
	repo     *postgres.ACSRepo
	adapters map[string]AdapterConstructor
}

type AdapterConstructor func(ctrl *domain.ACSController) (Adapter, error)

func NewManager(repo *postgres.ACSRepo) *Manager {
	m := &Manager{
		repo:     repo,
		adapters: make(map[string]AdapterConstructor),
	}

	// Регистрируем адаптеры
	m.Register("hikvision", NewHikvisionAdapter)
	m.Register("dahua", NewDahuaAdapter)
	m.Register("promwad", NewPromwadAdapter)
	m.Register("skud", NewSkudAdapter)

	return m
}

func (m *Manager) Register(vendor string, constructor AdapterConstructor) {
	m.adapters[vendor] = constructor
}

func (m *Manager) Repo() *postgres.ACSRepo {
	return m.repo
}

func (m *Manager) GetAdapter(vendor string, ctrl *domain.ACSController) (Adapter, error) {
	constructor, ok := m.adapters[vendor]
	if !ok {
		return nil, fmt.Errorf("unknown vendor: %s", vendor)
	}
	return constructor(ctrl)
}
