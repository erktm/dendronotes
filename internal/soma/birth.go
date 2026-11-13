package soma

import "os"

// births reads when entries were created, where the system keeps that
// (statx on Linux, the birth time of stat on macOS). Elsewhere, and on file
// systems without it, the time of the last change stands in.
type births struct {
	root *os.Root
	// The folder whose entries are being read (Linux reads them through it).
	dir string
	f   *os.File
}

func (b *births) close() {
	if b.f != nil {
		b.f.Close()
		b.f = nil
	}
}
