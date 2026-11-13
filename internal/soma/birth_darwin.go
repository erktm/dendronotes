package soma

import (
	"io/fs"
	"syscall"
)

// of returns when p was created (Unix milliseconds).
func (b *births) of(_ string, info fs.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Birthtimespec.Sec > 0 {
		return st.Birthtimespec.Sec*1000 + st.Birthtimespec.Nsec/1e6
	}
	return info.ModTime().UnixMilli()
}
