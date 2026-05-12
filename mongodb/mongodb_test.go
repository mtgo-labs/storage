package mongodb_test

import (
	"testing"

	"github.com/mtgo-labs/storage/mongodb"
)

func TestMongoDBImplements(t *testing.T) {
	// Compile-time interface check only.
	// Integration test requires a running MongoDB instance.
	_ = mongodb.Open
}
