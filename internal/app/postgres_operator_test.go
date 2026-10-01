package app

import (
	"context"
	"io"
	"testing"
)

func TestPostgresOperatorRejectsUnsafeInputs(t *testing.T) {
	for _, args := range [][]string{
		{}, {"import"}, {"import", "--database", "postgres:/private/dsn"},
		{"import", "--source", "/source", "--database", "/sqlite"},
		{"install-register", "--source", "/source", "--database", "postgres:/private/dsn"},
		{"install-register", "--database", "postgres:/private/dsn", "extra"},
		{"activate", "--database", "postgres:/private/dsn"},
	} {
		if err := executePostgresOperator(context.Background(), args, io.Discard); err == nil {
			t.Fatalf("accepted invalid operation %v", args)
		}
	}
}
