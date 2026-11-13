//go:build !linux && !darwin

package soma

import "io/fs"

// of returns the time of the last change: this system keeps no birth time
// that dendronotes reads.
func (b *births) of(_ string, info fs.FileInfo) int64 {
	return info.ModTime().UnixMilli()
}
