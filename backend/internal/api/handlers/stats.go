package handlers

import (
	"context"
	"net/http"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
)

type StatsHandler struct {
	db *pgxpool.Pool
}

func NewStatsHandler(db *pgxpool.Pool) *StatsHandler {
	return &StatsHandler{db: db}
}

func (h *StatsHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats := domain.Stats{}

	// Количество камер
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM cameras`).Scan(&stats.TotalCameras)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM cameras WHERE status = 'online'`).Scan(&stats.OnlineCameras)

	// События за 24 часа
	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM detection_events WHERE timestamp > NOW() - INTERVAL '24 hours'`,
	).Scan(&stats.TotalEvents24h)

	// Место на диске
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		stats.DiskTotalGB = float64(stat.Blocks*uint64(stat.Bsize)) / 1e9
		stats.DiskUsedGB = float64((stat.Blocks-stat.Bfree)*uint64(stat.Bsize)) / 1e9
	}

	// СКУД
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM acs_controllers`).Scan(&stats.ACSTotal)
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM acs_controllers WHERE status = 'online'`).Scan(&stats.ACSOnline)

	writeJSON(w, http.StatusOK, stats)
}

var _ = context.Background
