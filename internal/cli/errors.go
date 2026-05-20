package cli

import "errors"

// errorsAs is a thin wrapper to keep import of "errors" out of files that
// already need other helpers; it exists purely for readability at call sites.
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
