package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"greenlight.aida.kz/internal/validator"
	"time"
)

type Anime struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"-"`
	Title     string    `json:"title"`
	Year      int32     `json:"year,omitempty"`
	Runtime   Runtime   `json:"runtime,omitempty"`
	Genres    []string  `json:"genres,omitempty"`
	Version   int32     `json:"version"`
}

func ValidateAnime(v *validator.Validator, anime *Anime) {
	v.Check(anime.Title != "", "title", "must be provided")
	v.Check(len(anime.Title) <= 500, "title", "must not be more than 500 bytes long")
	v.Check(anime.Year != 0, "year", "must be provided")
	v.Check(anime.Year >= 1888, "year", "must be greater than 1888")
	v.Check(anime.Year <= int32(time.Now().Year()), "year", "must not be in the future")
	v.Check(anime.Runtime != 0, "runtime", "must be provided")
	v.Check(anime.Runtime > 0, "runtime", "must be a positive integer")
	v.Check(anime.Genres != nil, "genres", "must be provided")
	v.Check(len(anime.Genres) >= 1, "genres", "must contain at least 1 genre")
	v.Check(len(anime.Genres) <= 5, "genres", "must not contain more than 5 genres")
	v.Check(validator.Unique(anime.Genres), "genres", "must not contain duplicate values")
}

type AnimeModel struct {
	DB *pgxpool.Pool
}

func (m AnimeModel) Insert(anime *Anime) error {

	query := `
INSERT INTO animes (title, year, runtime, genres)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at, version`

	args := []any{anime.Title, anime.Year, anime.Runtime, anime.Genres}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.DB.QueryRow(ctx, query, args...).Scan(&anime.ID, &anime.CreatedAt, &anime.Version)
}

func (m AnimeModel) Get(id int64) (*Anime, error) {
	// The PostgreSQL bigserial type that we're using for the anime ID starts
	// auto-incrementing at 1 by default, so we know that no animes will have ID values
	// less than that. To avoid making an unnecessary database call, we take a shortcut
	// and return an ErrRecordNotFound error straight away.
	if id < 1 {
		return nil, ErrRecordNotFound
	}
	// Define the SQL query for retrieving the anime data.
	query := `
SELECT id, created_at, title, year, runtime, genres, version
FROM animes
WHERE id = $1`
	// Declare a Anime struct to hold the data returned by the query.
	var anime Anime

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	// Importantly, use defer to make sure that we cancel the context before the Get()
	// method returns.
	defer cancel()

	err := m.DB.QueryRow(ctx, query, id).Scan(
		&anime.ID,
		&anime.CreatedAt,
		&anime.Title,
		&anime.Year,
		&anime.Runtime,
		&anime.Genres,
		&anime.Version,
	)
	// Handle any errors. If there was no matching anime found, Scan() will return
	// a sql.ErrNoRows error. We check for this and return our custom ErrRecordNotFound
	// error instead.
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}
	// Otherwise, return a pointer to the Anime struct.
	return &anime, nil
}

func (m AnimeModel) Update(anime *Anime) error {
	// Add the 'AND version = $6' clause to the SQL query.
	query := `
SELECT id, created_at, title, year, runtime, genres, version
FROM animes
WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', $1) OR $1 = '')
AND (genres @> $2 OR $2 = '{}')
ORDER BY id`

	args := []any{
		anime.Title,
		anime.Year,
		anime.Runtime,
		anime.Genres,
		anime.ID,
		anime.Version, // Add the expected anime version.
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Execute the SQL query. If no matching row could be found, we know the anime
	// version has changed (or the record has been deleted) and we return our custom
	// ErrEditConflict error.
	err := m.DB.QueryRow(ctx, query, args...).Scan(&anime.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}
	return nil
}

func (m AnimeModel) Delete(id int64) error {
	// Return an ErrRecordNotFound error if the anime ID is less than 1.
	if id < 1 {
		return ErrRecordNotFound
	}
	// Construct the SQL query to delete the record.
	query := `
DELETE FROM animes
WHERE id = $1`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Execute the SQL query using the Exec() method, passing in the id variable as
	// the value for the placeholder parameter. The Exec() method returns a sql.Result
	// object.
	result, err := m.DB.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	// Call the RowsAffected() method on the sql.Result object to get the number of rows
	// affected by the query.
	rowsAffected := result.RowsAffected()
	if err != nil {
		return err
	}
	// If no rows were affected, we know that the animes table didn't contain a record
	// with the provided ID at the moment we tried to delete it. In that case we
	// return an ErrRecordNotFound error.
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (m AnimeModel) GetAll(title string, genres []string, filters Filters) ([]*Anime, Metadata, error) {
	// Construct the SQL query to retrieve all anime records.
	query := fmt.Sprintf(`
SELECT count(*) OVER(), id, created_at, title, year, runtime, genres, version
FROM animes
WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', $1) OR $1 = '')
AND (genres @> $2 OR $2 = '{}')
ORDER BY %s %s, id ASC
LIMIT $3 OFFSET $4`, filters.sortColumn(), filters.sortDirection())

	// Create a context with a 3-second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{title, genres, filters.limit(), filters.offset()}
	// Use QueryContext() to execute the query. This returns a sql.Rows resultset
	// containing the result.
	rows, err := m.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}

	defer rows.Close()
	// Initialize an empty slice to hold the anime data.
	animes := []*Anime{}
	totalRecords := 0
	// Use rows.Next to iterate through the rows in the resultset.
	for rows.Next() {
		// Initialize an empty Anime struct to hold the data for an individual anime.
		var anime Anime
		// Scan the values from the row into the Anime struct. Again, note that we're
		// using the pq.Array() adapter on the genres field here.
		err := rows.Scan(
			&totalRecords,
			&anime.ID,
			&anime.CreatedAt,
			&anime.Title,
			&anime.Year,
			&anime.Runtime,
			&anime.Genres,
			&anime.Version,
		)
		if err != nil {
			return nil, Metadata{}, err
		}
		// Add the Anime struct to the slice.
		animes = append(animes, &anime)
	}
	// When the rows.Next() loop has finished, call rows.Err() to retrieve any error
	// that was encountered during the iteration.
	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)
	// If everything went OK, then return the slice of animes.
	return animes, metadata, nil

}
