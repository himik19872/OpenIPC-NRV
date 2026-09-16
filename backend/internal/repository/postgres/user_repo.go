package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nvr/backend/internal/domain"
)

type UserRepo struct {
	db *pgxpool.Pool
}

func NewUserRepo(db *pgxpool.Pool) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	var perms []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, username, password_hash, role, permissions, created_at
		FROM users WHERE username = $1
	`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &perms, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if perms != nil {
		json.Unmarshal(perms, &u.Permissions)
	}
	return &u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var u domain.User
	var perms []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, username, password_hash, role, permissions, created_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &perms, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if perms != nil {
		json.Unmarshal(perms, &u.Permissions)
	}
	return &u, nil
}
