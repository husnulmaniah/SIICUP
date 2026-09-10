// Package assets embeds small static binary assets used by the backend
// (currently just the instansi logo used on the printable leave forms) so
// they ship inside the compiled binary and need no separate file on disk at
// runtime/deploy time.
package assets

import _ "embed"

//go:embed logo.png
var LogoPNG []byte
