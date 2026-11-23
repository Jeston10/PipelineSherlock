// Package migrations embeds the versioned SQL migration files so the server
// binary carries its own schema and can migrate without external files.
package migrations

import "embed"

// FS contains every *.sql migration file in this directory. Files follow the
// naming convention <version>_<name>.<up|down>.sql, e.g. 0001_init.up.sql.
//
//go:embed *.sql
var FS embed.FS
