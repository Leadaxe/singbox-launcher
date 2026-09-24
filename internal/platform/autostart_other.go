//go:build !windows

package platform

// ReadAutostart — только Windows.
func ReadAutostart() (entry AutostartEntry, found bool, err error) {
	return AutostartEntry{}, false, ErrAutostartNotSupported
}

// WriteAutostart — только Windows.
func WriteAutostart(e AutostartEntry) error {
	_ = e
	return ErrAutostartNotSupported
}

// DeleteAutostart — только Windows.
func DeleteAutostart() error { return ErrAutostartNotSupported }
