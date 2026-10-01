// Package db contains immutable migration assets, not database access.
package db

import "embed"

// Migrations is consumed exclusively by the SQLite adapter.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// PostgresMigrations mirrors the schema-12 business contract.
//
//go:embed postgres/*.sql
var PostgresMigrations embed.FS
