//go:build payload

package parts

import (
	"embed"
	"io/fs"
)

// payload/api<N>/ holds the native helpers built for each NativeAPI a board
// names; payload/ itself holds what one API does not decide.
//
//go:embed payload
var payload embed.FS

var native fs.FS = payload
