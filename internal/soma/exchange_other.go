//go:build !linux && !darwin

package soma

import (
	"errors"
	"os"
)

// exchangeAt would swap two entries in one step; writes rename instead here.
func exchangeAt(*os.File, string, string) error { return errors.ErrUnsupported }
