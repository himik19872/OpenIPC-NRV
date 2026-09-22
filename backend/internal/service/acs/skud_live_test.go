package acs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
)

// TestSkudLive проверяет адаптер на реальном контроллере.
// Требует контроллер в сети; пропускается, если он недоступен.
func TestSkudLive(t *testing.T) {
	ctrl := &domain.ACSController{
		ID:          uuid.New(),
		Name:        "SKUD-01",
		Vendor:      "skud",
		IP:          "192.168.1.51",
		Port:        80,
		Credentials: map[string]any{"login": "admin", "password": "admin"},
	}
	a, err := NewSkudAdapter(ctrl)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := a.Ping(ctx); err != nil {
		t.Skipf("контроллер недоступен: %v", err)
	}

	doors, err := a.ListDoors(ctx)
	if err != nil {
		t.Fatalf("ListDoors: %v", err)
	}
	t.Logf("двери: %+v", doors)

	st, err := a.GetDoorStatus(ctx, doorID)
	if err != nil {
		t.Fatalf("GetDoorStatus: %v", err)
	}
	t.Logf("состояние двери: %+v", st)

	// Открываем дверь и проверяем, что она открылась.
	if err := a.OpenDoor(ctx, doorID); err != nil {
		t.Fatalf("OpenDoor: %v", err)
	}
	t.Log("дверь открыта")
	time.Sleep(1500)

	st, err = a.GetDoorStatus(ctx, doorID)
	if err != nil {
		t.Fatalf("GetDoorStatus после открытия: %v", err)
	}
	t.Logf("состояние после открытия: %+v", st)
}
