package soma

import (
	"io/fs"
	"path"

	"golang.org/x/sys/unix"
)

// of returns when p was created (Unix milliseconds). The entry is read by
// name relative to its folder, opened through the root, so nothing outside
// the soma is looked at.
func (b *births) of(p string, info fs.FileInfo) int64 {
	dir, name := path.Dir(p), path.Base(p)
	if b.f == nil || b.dir != dir {
		b.close()
		f, err := b.root.Open(dir)
		if err != nil {
			return info.ModTime().UnixMilli()
		}
		b.f, b.dir = f, dir
	}
	var st unix.Statx_t
	err := unix.Statx(int(b.f.Fd()), name, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_DONT_SYNC, unix.STATX_BTIME, &st)
	if err != nil || st.Mask&unix.STATX_BTIME == 0 || st.Btime.Sec <= 0 {
		return info.ModTime().UnixMilli()
	}
	return st.Btime.Sec*1000 + int64(st.Btime.Nsec)/1e6
}
