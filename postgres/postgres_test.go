package postgres_test

import (
	"testing"

	"github.com/mtgo-labs/storage/postgres"
)

func TestPostgresImplements(t *testing.T) {
	// Compile-time interface check only.
	// Integration test requires a running PostgreSQL instance.
	_ = postgres.Open
}
