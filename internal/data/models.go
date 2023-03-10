package data

import (
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRecordNotFound = errors.New("no rows in result set")
	ErrEditConflict   = errors.New("edit conflict")
)

type Models struct {
	Animes      AnimeModel
	Permissions PermissionModel
	Tokens      TokenModel
	Users       UserModel
}

func NewModels(db *pgxpool.Pool) Models {
	return Models{
		Animes:      AnimeModel{DB: db},
		Permissions: PermissionModel{DB: db},
		Tokens:      TokenModel{DB: db},
		Users:       UserModel{DB: db},
	}
}
