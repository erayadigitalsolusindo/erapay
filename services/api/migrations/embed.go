// Package migrations menyematkan file SQL ke dalam binary.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
