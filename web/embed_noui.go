//go:build !ui

package web

import "embed"

var Assets embed.FS

const Embedded = false
