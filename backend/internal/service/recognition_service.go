package service

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/rs/zerolog/log"
)

// RecognitionService сопоставляет обнаруженные лица и номера со справочниками.
//
// Справочники меняются редко, а сравнивать приходится на каждое событие,
// поэтому они держатся в кэше и периодически перечитываются из БД.
// Сравнение лиц — полный перебор по косинусной близости: записей десятки,
// поэтому индекс для поиска ближайшего соседа не нужен.
type RecognitionService struct {
	repo RecognitionRepository

	cacheTTL time.Duration

	mu       sync.RWMutex
	faces    []faceEntry
	plates   map[string]domain.KnownPlate
	facesAt  time.Time
	platesAt time.Time
}

// faceEntry — лицо в кэше вместе с эмбеддингом.
type faceEntry struct {
	id        uuid.UUID
	name      string
	blocked   bool
	embedding []float32
	// norm — заранее посчитанная норма вектора: экономит вычисления
	// при сравнении с каждым событием.
	norm float64
}

// RecognitionRepository — доступ к справочникам.
type RecognitionRepository interface {
	FaceEmbeddings(ctx context.Context) (map[string][]float32, error)
	ListFaces(ctx context.Context, withDisabled bool) ([]domain.KnownFace, error)
	ListPlates(ctx context.Context, withDisabled bool) ([]domain.KnownPlate, error)
	GetRecognitionSettings(ctx context.Context) (*domain.RecognitionSettings, error)
}

func NewRecognitionService(repo RecognitionRepository) *RecognitionService {
	return &RecognitionService{
		repo:     repo,
		cacheTTL: 30 * time.Second,
		plates:   make(map[string]domain.KnownPlate),
	}
}

// MatchFace ищет лицо в справочнике по эмбеддингу.
//
// Возвращает MatchUnknown, если распознавание выключено или совпадений нет.
func (s *RecognitionService) MatchFace(ctx context.Context, embedding []float32) (res domain.MatchResult) {
	if len(embedding) == 0 {
		return domain.MatchResult{Type: domain.MatchUnknown}
	}

	cfg, err := s.repo.GetRecognitionSettings(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("распознавание лиц: не удалось прочитать настройки")
		return domain.MatchResult{Type: domain.MatchUnknown}
	}
	if !cfg.Faces.Enabled {
		return domain.MatchResult{Type: domain.MatchUnknown}
	}

	best := s.bestFaceMatch(embedding)
	if best == nil {
		return domain.MatchResult{Type: domain.MatchUnknown}
	}
	// Порог сравниваем с косинусной близостью (0..1): чем больше, тем похожее.
	if best.similarity < cfg.Faces.Threshold {
		return domain.MatchResult{Type: domain.MatchUnknown}
	}

	out := domain.MatchResult{
		Type: domain.MatchKnown,
		ID:   best.id,
		Name: best.name,
	}
	if best.blocked && cfg.Faces.AlertBlocked {
		out.Type = domain.MatchBlocked
	}
	log.Debug().
		Str("name", best.name).
		Float64("similarity", best.similarity).
		Msg("распознавание лиц: найдено совпадение")
	return out
}

// MatchPlate ищет номер в справочнике.
func (s *RecognitionService) MatchPlate(ctx context.Context, plate string) domain.MatchResult {
	if plate == "" {
		return domain.MatchResult{Type: domain.MatchUnknown}
	}

	cfg, err := s.repo.GetRecognitionSettings(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("распознавание номеров: не удалось прочитать настройки")
		return domain.MatchResult{Type: domain.MatchUnknown}
	}
	if !cfg.Plates.Enabled {
		return domain.MatchResult{Type: domain.MatchUnknown}
	}

	found, ok := s.findPlate(ctx, plate)
	if !ok {
		return domain.MatchResult{Type: domain.MatchUnknown}
	}

	out := domain.MatchResult{
		Type: domain.MatchKnown,
		ID:   found.ID,
		Name: found.Plate,
	}
	if found.IsBlocked && cfg.Plates.AlertBlocked {
		out.Type = domain.MatchBlocked
		out.Name = found.Plate + " (" + found.Owner + ")"
	}
	return out
}

// --- Кэш справочников ---

type faceMatch struct {
	id         uuid.UUID
	name       string
	blocked    bool
	similarity float64
}

// bestFaceMatch ищет самое похожее лицо в справочнике.
func (s *RecognitionService) bestFaceMatch(embedding []float32) *faceMatch {
	entries := s.loadFaces()
	if len(entries) == 0 {
		return nil
	}

	targetNorm := vectorNorm(embedding)
	if targetNorm == 0 {
		return nil
	}

	var best *faceMatch
	for _, e := range entries {
		if len(e.embedding) != len(embedding) {
			// Эмбеддинги разных моделей несравнимы — пропускаем.
			continue
		}
		sim := cosineSimilarity(embedding, targetNorm, e.embedding, e.norm)
		if best == nil || sim > best.similarity {
			best = &faceMatch{id: e.id, name: e.name, blocked: e.blocked, similarity: sim}
		}
	}
	return best
}

// findPlate ищет номер в кэше справочника.
func (s *RecognitionService) findPlate(ctx context.Context, plate string) (domain.KnownPlate, bool) {
	s.refreshPlatesIfStale(ctx)
	norm := postgres.NormalizePlate(plate)

	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plates[norm]
	return p, ok
}

// loadFaces возвращает кэш лиц, обновляя его при необходимости.
func (s *RecognitionService) loadFaces() []faceEntry {
	s.mu.RLock()
	expired := time.Since(s.facesAt) > s.cacheTTL
	faces := s.faces
	s.mu.RUnlock()

	if !expired {
		return faces
	}
	return s.refreshFaces()
}

// refreshFaces перечитывает справочник лиц и кэширует эмбеддинги.
func (s *RecognitionService) refreshFaces() []faceEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	faces, err := s.repo.ListFaces(ctx, false)
	if err != nil {
		log.Warn().Err(err).Msg("не удалось обновить кэш лиц")
		// Отдаём старый кэш: без справочника распознавание просто не сработает,
		// но события продолжат сохраняться.
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.faces
	}

	// Эмбеддинги запрашиваем отдельным запросом: ListFaces отдаёт только
	// признак их наличия, чтобы не гонять большие массивы в обычный список.
	embeddings, err := s.repo.FaceEmbeddings(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("не удалось загрузить эмбеддинги лиц")
		embeddings = map[string][]float32{}
	}

	entries := make([]faceEntry, 0, len(faces))
	for _, f := range faces {
		emb := embeddings[f.ID.String()]
		if len(emb) == 0 {
			continue // лицо без биометрии в сравнении не участвует
		}
		entries = append(entries, faceEntry{
			id:        f.ID,
			name:      f.Name,
			blocked:   f.IsBlocked,
			embedding: emb,
			norm:      vectorNorm(emb),
		})
	}

	s.mu.Lock()
	s.faces = entries
	s.facesAt = time.Now()
	s.mu.Unlock()

	return entries
}

// refreshPlatesIfStale перечитывает справочник номеров при устаревании кэша.
func (s *RecognitionService) refreshPlatesIfStale(ctx context.Context) {
	s.mu.RLock()
	stale := time.Since(s.platesAt) > s.cacheTTL
	s.mu.RUnlock()
	if !stale {
		return
	}

	plates, err := s.repo.ListPlates(ctx, false)
	if err != nil {
		log.Warn().Err(err).Msg("не удалось обновить кэш номеров")
		return
	}

	next := make(map[string]domain.KnownPlate, len(plates))
	for _, p := range plates {
		next[p.PlateNorm] = p
	}

	s.mu.Lock()
	s.plates = next
	s.platesAt = time.Now()
	s.mu.Unlock()
}

// --- Математика сравнения лиц ---

// cosineSimilarity считает косинусную близость двух векторов.
// Нормы передаются заранее: они не меняются между сравнениями.
func cosineSimilarity(a []float32, normA float64, b []float32, normB float64) float64 {
	if normA == 0 || normB == 0 {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot / (normA * normB)
}

// vectorNorm считает евклидову норму вектора.
func vectorNorm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}
