// Package migrations embeds all SQL migration files for auth-svc.
// golang-migrate reads from this FS at startup to apply pending migrations.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
