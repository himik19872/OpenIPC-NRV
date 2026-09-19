package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
)

// RecognitionRepo — справочники известных лиц и автомобильных номеров.
//
// Эти списки нужны, чтобы отличать «своих» от посторонних: событие
// сопоставляется со справочником, и результат попадает в detection_events.
type RecognitionRepo struct {
	db *pgxpool.Pool
}

func NewRecognitionRepo(db *pgxpool.Pool) *RecognitionRepo {
	return &RecognitionRepo{db: db}
}

// ErrNotFound возвращается, когда запись справочника не найдена.
var ErrNotFound = errors.New("запись не найдена")

// --- Лица ---

// ListFaces возвращает справочник лиц. withDisabled управляет показом
// отключённых записей — по умолчанию они скрыты.
func (r *RecognitionRepo) ListFaces(ctx context.Context, withDisabled bool) ([]domain.KnownFace, error) {
	q := `
		SELECT id, name, note, COALESCE(photo_path,''), is_blocked, enabled,
		       created_at, updated_at, embedding IS NOT NULL
		FROM known_faces`
	if !withDisabled {
		q += ` WHERE enabled = true`
	}
	q += ` ORDER BY name`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list faces: %w", err)
	}
	defer rows.Close()

	out := make([]domain.KnownFace, 0)
	for rows.Next() {
		var f domain.KnownFace
		if err := rows.Scan(&f.ID, &f.Name, &f.Note, &f.PhotoPath, &f.IsBlocked,
			&f.Enabled, &f.CreatedAt, &f.UpdatedAt, &f.HasEmbedding); err != nil {
			return nil, fmt.Errorf("scan face: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetFace возвращает одну запись справочника лиц.
func (r *RecognitionRepo) GetFace(ctx context.Context, id uuid.UUID) (*domain.KnownFace, error) {
	var f domain.KnownFace
	err := r.db.QueryRow(ctx, `
		SELECT id, name, note, COALESCE(photo_path,''), is_blocked, enabled,
		       created_at, updated_at, embedding IS NOT NULL
		FROM known_faces WHERE id = $1`, id,
	).Scan(&f.ID, &f.Name, &f.Note, &f.PhotoPath, &f.IsBlocked, &f.Enabled,
		&f.CreatedAt, &f.UpdatedAt, &f.HasEmbedding)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get face: %w", err)
	}
	return &f, nil
}

// CreateFace добавляет лицо в справочник.
func (r *RecognitionRepo) CreateFace(ctx context.Context, f *domain.KnownFace, embedding []float32) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO known_faces (name, note, photo_path, embedding, is_blocked, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`,
		f.Name, f.Note, nullIfEmpty(f.PhotoPath), embeddingToFloats(embedding),
		f.IsBlocked, true,
	).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

// UpdateFace обновляет поля записи справочника. nil означает «не менять».
func (r *RecognitionRepo) UpdateFace(ctx context.Context, id uuid.UUID, req domain.UpdateKnownFaceRequest) error {
	sets := []string{}
	args := []any{}
	i := 1

	add := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}

	if req.Name != nil {
		add("name", *req.Name)
	}
	if req.Note != nil {
		add("note", *req.Note)
	}
	if req.IsBlocked != nil {
		add("is_blocked", *req.IsBlocked)
	}
	if req.Enabled != nil {
		add("enabled", *req.Enabled)
	}
	if req.Embedding != nil {
		add("embedding", embeddingToFloats(req.Embedding))
	}

	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = now()")
	args = append(args, id)

	q := fmt.Sprintf("UPDATE known_faces SET %s WHERE id = $%d",
		strings.Join(sets, ", "), i)
	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update face: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateFacePhoto сохраняет путь к эталонному снимку лица.
func (r *RecognitionRepo) UpdateFacePhoto(ctx context.Context, id uuid.UUID, photoPath string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE known_faces SET photo_path = $1, updated_at = now() WHERE id = $2`,
		photoPath, id)
	if err != nil {
		return fmt.Errorf("update face photo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteFace удаляет лицо из справочника.
func (r *RecognitionRepo) DeleteFace(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM known_faces WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete face: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FaceEmbeddings возвращает эмбеддинги активных лиц и их имена.
// Используется детектором для сравнения при распознавании.
func (r *RecognitionRepo) FaceEmbeddings(ctx context.Context) (map[string][]float32, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, embedding
		FROM known_faces
		WHERE enabled = true AND embedding IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("load face embeddings: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]float32)
	for rows.Next() {
		var id, name string
		var emb []float32
		if err := rows.Scan(&id, &name, &emb); err != nil {
			return nil, fmt.Errorf("scan embedding: %w", err)
		}
		out[id] = emb
	}
	return out, rows.Err()
}

// --- Номера ---

// ListPlates возвращает справочник номеров.
func (r *RecognitionRepo) ListPlates(ctx context.Context, withDisabled bool) ([]domain.KnownPlate, error) {
	q := `
		SELECT id, plate, plate_norm, owner, note, COALESCE(photo_path,''),
		       is_blocked, enabled, created_at, updated_at
		FROM known_plates`
	if !withDisabled {
		q += ` WHERE enabled = true`
	}
	q += ` ORDER BY plate`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list plates: %w", err)
	}
	defer rows.Close()

	out := make([]domain.KnownPlate, 0)
	for rows.Next() {
		var p domain.KnownPlate
		if err := rows.Scan(&p.ID, &p.Plate, &p.PlateNorm, &p.Owner, &p.Note,
			&p.PhotoPath, &p.IsBlocked, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan plate: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPlate возвращает одну запись справочника номеров.
func (r *RecognitionRepo) GetPlate(ctx context.Context, id uuid.UUID) (*domain.KnownPlate, error) {
	var p domain.KnownPlate
	err := r.db.QueryRow(ctx, `
		SELECT id, plate, plate_norm, owner, note, COALESCE(photo_path,''),
		       is_blocked, enabled, created_at, updated_at
		FROM known_plates WHERE id = $1`, id,
	).Scan(&p.ID, &p.Plate, &p.PlateNorm, &p.Owner, &p.Note, &p.PhotoPath,
		&p.IsBlocked, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get plate: %w", err)
	}
	return &p, nil
}

// CreatePlate добавляет номер в справочник.
// Номер нормализуется автоматически — сравнивать будем по нормализованному виду.
func (r *RecognitionRepo) CreatePlate(ctx context.Context, p *domain.KnownPlate) error {
	p.PlateNorm = NormalizePlate(p.Plate)
	return r.db.QueryRow(ctx, `
		INSERT INTO known_plates (plate, plate_norm, owner, note, photo_path, is_blocked, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		RETURNING id, created_at, updated_at`,
		p.Plate, p.PlateNorm, p.Owner, p.Note, nullIfEmpty(p.PhotoPath), p.IsBlocked,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// UpdatePlate обновляет запись справочника номеров.
func (r *RecognitionRepo) UpdatePlate(ctx context.Context, id uuid.UUID, req domain.UpdateKnownPlateRequest) error {
	sets := []string{}
	args := []any{}
	i := 1

	add := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}

	// Номер меняем вместе с нормализованной формой, иначе поиск развалится.
	if req.Plate != nil {
		add("plate", *req.Plate)
		add("plate_norm", NormalizePlate(*req.Plate))
	}
	if req.Owner != nil {
		add("owner", *req.Owner)
	}
	if req.Note != nil {
		add("note", *req.Note)
	}
	if req.IsBlocked != nil {
		add("is_blocked", *req.IsBlocked)
	}
	if req.Enabled != nil {
		add("enabled", *req.Enabled)
	}

	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = now()")
	args = append(args, id)

	q := fmt.Sprintf("UPDATE known_plates SET %s WHERE id = $%d",
		strings.Join(sets, ", "), i)
	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update plate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePlatePhoto сохраняет путь к снимку автомобиля.
func (r *RecognitionRepo) UpdatePlatePhoto(ctx context.Context, id uuid.UUID, photoPath string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE known_plates SET photo_path = $1, updated_at = now() WHERE id = $2`,
		photoPath, id)
	if err != nil {
		return fmt.Errorf("update plate photo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePlate удаляет номер из справочника.
func (r *RecognitionRepo) DeletePlate(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM known_plates WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete plate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindPlate ищет номер в справочнике по нормализованному виду.
// Возвращает nil, если совпадений нет — это не ошибка.
func (r *RecognitionRepo) FindPlate(ctx context.Context, plate string) (*domain.KnownPlate, error) {
	var p domain.KnownPlate
	err := r.db.QueryRow(ctx, `
		SELECT id, plate, plate_norm, owner, note, COALESCE(photo_path,''),
		       is_blocked, enabled, created_at, updated_at
		FROM known_plates WHERE plate_norm = $1 AND enabled = true`,
		NormalizePlate(plate),
	).Scan(&p.ID, &p.Plate, &p.PlateNorm, &p.Owner, &p.Note, &p.PhotoPath,
		&p.IsBlocked, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find plate: %w", err)
	}
	return &p, nil
}

// Counts возвращает размеры справочников — для сводки в интерфейсе.
func (r *RecognitionRepo) Counts(ctx context.Context) (faces int, plates int, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM known_faces WHERE enabled = true),
		       (SELECT count(*) FROM known_plates WHERE enabled = true)`,
	).Scan(&faces, &plates)
	return faces, plates, err
}

// --- Настройки распознавания ---

// GetRecognitionSettings читает настройки распознавания из server_settings.
// Отсутствующие ключи дают значения по умолчанию (распознавание выключено).
func (r *RecognitionRepo) GetRecognitionSettings(ctx context.Context) (*domain.RecognitionSettings, error) {
	out := &domain.RecognitionSettings{
		Faces:  domain.FaceRecognitionSettings{Threshold: 0.45, SnapshotUnknown: true, AlertBlocked: true},
		Plates: domain.PlateRecognitionSettings{Threshold: 0.75, Region: "ru", SnapshotUnknown: true, AlertBlocked: true},
	}

	rows, err := r.db.Query(ctx, `
		SELECT key, value FROM server_settings
		WHERE key IN ('face_recognition', 'plate_recognition')`)
	if err != nil {
		return nil, fmt.Errorf("get recognition settings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, fmt.Errorf("scan recognition settings: %w", err)
		}
		switch key {
		case "face_recognition":
			if err := decodeJSON(raw, &out.Faces); err != nil {
				return nil, err
			}
		case "plate_recognition":
			if err := decodeJSON(raw, &out.Plates); err != nil {
				return nil, err
			}
		}
	}
	return out, rows.Err()
}

// UpdateRecognitionSettings сохраняет настройки распознавания.
func (r *RecognitionRepo) UpdateRecognitionSettings(ctx context.Context, req domain.UpdateRecognitionSettingsRequest) (*domain.RecognitionSettings, error) {
	cur, err := r.GetRecognitionSettings(ctx)
	if err != nil {
		return nil, err
	}

	// Дополняем текущие значения переданными полями: в запросе клиент
	// обычно присылает только изменённую часть.
	if req.Faces != nil {
		cur.Faces = *req.Faces
	}
	if req.Plates != nil {
		cur.Plates = *req.Plates
	}

	for key, val := range map[string]any{
		"face_recognition":  cur.Faces,
		"plate_recognition": cur.Plates,
	} {
		if _, err := r.db.Exec(ctx, `
			INSERT INTO server_settings (key, value, updated_at)
			VALUES ($1, $2, now())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
			key, val); err != nil {
			return nil, fmt.Errorf("save %s: %w", key, err)
		}
	}
	return cur, nil
}

// NormalizePlate приводит номер к виду, пригодному для сравнения:
// только буквы и цифры, верхний регистр.
//
// OCR и человек вводят номера по-разному («А 123 ВС-77», «а123вс77»),
// поэтому сравнение идёт по этой форме.
func NormalizePlate(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		switch {
		case r >= 'А' && r <= 'Я', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// embeddingToFloats превращает эмбеддинг в значение для колонки REAL[].
// nil означает «биометрия не задана» — в БД попадёт NULL.
func embeddingToFloats(emb []float32) any {
	if len(emb) == 0 {
		return nil
	}
	out := make([]float64, len(emb))
	for i, v := range emb {
		out[i] = float64(v)
	}
	return out
}

// nullIfEmpty возвращает nil для пустой строки: в БД попадёт NULL,
// а не пустая строка. Так проще отличать «файла нет» от «файл есть».
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// decodeJSON разбирает JSONB-значение настройки в структуру.
// Пустое значение считается «настройка не задана» и не является ошибкой.
func decodeJSON(raw []byte, out any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}
