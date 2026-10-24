//go:build ui

package web

import "embed"

//go:embed all:build
var Assets embed.FS

const Embedded = true
