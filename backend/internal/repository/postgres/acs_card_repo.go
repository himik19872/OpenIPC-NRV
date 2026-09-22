package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
	"github.com/rs/zerolog/log"
)

// ACSCardRepo — хранилище карт доступа на сервере.
//
// Сервер считается источником истины: карты заводятся здесь, а на
// контроллеры выдаются отдельной операцией. Это позволяет держать единый
// справочник карт для нескольких контроллеров и не собирать базу заново
// при замене устройства.
type ACSCardRepo struct {
	db *pgxpool.Pool
}

func NewACSCardRepo(db *pgxpool.Pool) *ACSCardRepo {
	return &ACSCardRepo{db: db}
}

// ListCards возвращает карты. Если controllerID задан, список
// ограничивается одним контроллером.
func (r *ACSCardRepo) ListCards(ctx context.Context, controllerID *uuid.UUID) ([]domain.ACSCard, error) {
	query := `
		SELECT id, COALESCE(controller_id, '00000000-0000-0000-0000-000000000000'::uuid),
		       facility, card, name, grp, access, active
		FROM acs_cards
	`
	args := []any{}
	if controllerID != nil {
		query += ` WHERE controller_id = $1`
		args = append(args, *controllerID)
	}
	query += ` ORDER BY name, facility, card`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]domain.ACSCard, 0)
	for rows.Next() {
		var c domain.ACSCard
		if err := rows.Scan(&c.ID, &c.ControllerID, &c.Facility, &c.CardNumber,
			&c.Name, &c.Group, &c.Access, &c.Active); err != nil {
			log.Error().Err(err).Msg("не удалось прочитать карту СКУД")
			continue
		}
		cards = append(cards, c)
	}
	return cards, nil
}

// GetCard возвращает карту по id.
func (r *ACSCardRepo) GetCard(ctx context.Context, id uuid.UUID) (*domain.ACSCard, error) {
	var c domain.ACSCard
	err := r.db.QueryRow(ctx, `
		SELECT id, COALESCE(controller_id, '00000000-0000-0000-0000-000000000000'::uuid),
		       facility, card, name, grp, access, active
		FROM acs_cards WHERE id = $1
	`, id).Scan(&c.ID, &c.ControllerID, &c.Facility, &c.CardNumber,
		&c.Name, &c.Group, &c.Access, &c.Active)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpsertCard создаёт карту или обновляет существующую с той же парой
// facility+card на том же контроллере.
//
// Именно upsert, а не INSERT: одна и та же карта может быть заведена
// повторно (например, при импорте), и это не ошибка — нужно обновить
// запись, а не падать.
func (r *ACSCardRepo) UpsertCard(ctx context.Context, c *domain.ACSCard) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	var controllerID any
	if c.ControllerID != uuid.Nil {
		controllerID = c.ControllerID
	}

	return r.db.QueryRow(ctx, `
		INSERT INTO acs_cards (id, controller_id, facility, card, name, grp, access, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (controller_id, facility, card) DO UPDATE
		SET name = EXCLUDED.name,
		    grp = EXCLUDED.grp,
		    access = EXCLUDED.access,
		    active = EXCLUDED.active,
		    updated_at = now()
		RETURNING id
	`, c.ID, controllerID, c.Facility, c.CardNumber, c.Name, c.Group,
		c.Access, c.Active).Scan(&c.ID)
}

// DeleteCard удаляет карту по id.
func (r *ACSCardRepo) DeleteCard(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM acs_cards WHERE id = $1`, id)
	return err
}

// DeleteAllForController удаляет все карты контроллера.
func (r *ACSCardRepo) DeleteAllForController(ctx context.Context, controllerID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM acs_cards WHERE controller_id = $1`, controllerID)
	return err
}

// ReplaceAllForController перезаписывает базу карт контроллера.
//
// Используется при переносе базы с нового контроллера на сервер: список
// карт с устройства заменяет серверный целиком. В одной транзакции, чтобы
// при сбое не остаться с половиной справочника.
func (r *ACSCardRepo) ReplaceAllForController(ctx context.Context, controllerID uuid.UUID, cards []domain.ACSCard) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM acs_cards WHERE controller_id = $1`, controllerID); err != nil {
		return err
	}

	for _, c := range cards {
		if _, err := tx.Exec(ctx, `
			INSERT INTO acs_cards (id, controller_id, facility, card, name, grp, access, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (controller_id, facility, card) DO UPDATE
			SET name = EXCLUDED.name, grp = EXCLUDED.grp,
			    access = EXCLUDED.access, active = EXCLUDED.active,
			    updated_at = now()
		`, uuid.New(), controllerID, c.Facility, c.CardNumber, c.Name,
			c.Group, c.Access, c.Active); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// CountForController возвращает число карт контроллера.
func (r *ACSCardRepo) CountForController(ctx context.Context, controllerID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM acs_cards WHERE controller_id = $1`, controllerID).Scan(&n)
	return n, err
}

// FindByCard ищет карту по паре facility+card среди всех контроллеров.
// Нужно для сопоставления события доступа с владельцем карты.
func (r *ACSCardRepo) FindByCard(ctx context.Context, facility, card int) ([]domain.ACSCard, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, COALESCE(controller_id, '00000000-0000-0000-0000-000000000000'::uuid),
		       facility, card, name, grp, access, active
		FROM acs_cards WHERE facility = $1 AND card = $2
	`, facility, card)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]domain.ACSCard, 0, 1)
	for rows.Next() {
		var c domain.ACSCard
		if err := rows.Scan(&c.ID, &c.ControllerID, &c.Facility, &c.CardNumber,
			&c.Name, &c.Group, &c.Access, &c.Active); err != nil {
			continue
		}
		cards = append(cards, c)
	}
	return cards, nil
}
