package submissionmanager

import (
	"database/sql"
	"errors"
)

type sqlStore struct {
	db *sql.DB
}

func newSQLStore(db *sql.DB) (*sqlStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	return &sqlStore{db: db}, nil
}
