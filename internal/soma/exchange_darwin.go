package soma

import (
	"os"

	"golang.org/x/sys/unix"
)

// exchangeAt swaps the entries a and b of the folder d in one step.
func exchangeAt(d *os.File, a, b string) error {
	fd := int(d.Fd())
	return unix.RenameatxNp(fd, a, fd, b, unix.RENAME_SWAP)
}
