package db

import (
	"database/sql"
	"errors"

	_ "modernc.org/sqlite"
)

var (
	ErrDuplicateAlias         = errors.New("host alias already exists")
	ErrDuplicateDirectoryName = errors.New("directory name already exists")
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: database}
	if err := store.migrate(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
