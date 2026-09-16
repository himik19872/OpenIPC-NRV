package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/service"
)

type ScannerHandler struct {
	scanner *service.CameraScanner
}

func NewScannerHandler(scanner *service.CameraScanner) *ScannerHandler {
	return &ScannerHandler{scanner: scanner}
}

// Scan запускает сканирование подсети
func (h *ScannerHandler) Scan(w http.ResponseWriter, r *http.Request) {
	var req domain.ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.Subnet == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subnet is required"})
		return
	}

	result, err := h.scanner.Scan(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// Probe проверяет конкретный IP
func (h *ScannerHandler) Probe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IP       string `json:"ip"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	cam, err := h.scanner.ProbeSingle(r.Context(), req.IP, req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, cam)
}
