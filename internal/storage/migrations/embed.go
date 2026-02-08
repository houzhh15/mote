package migrations

import "embed"

//go:embed scripts/*.sql
var FS embed.FS
