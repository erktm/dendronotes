package soma

import (
	"os"

	"golang.org/x/sys/unix"
)

// exchangeAt swaps the entries a and b of the folder d in one step.
func exchangeAt(d *os.File, a, b string) error {
	fd := int(d.Fd())
	return unix.Renameat2(fd, a, fd, b, unix.RENAME_EXCHANGE)
}
