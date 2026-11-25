//go:build !windows

package bestfit

// Hits is every finding of the scan on Windows. Here it answers ErrNotWindows: no other system has the conversion.
func Hits() ([]Hit, error) { return nil, ErrNotWindows }
