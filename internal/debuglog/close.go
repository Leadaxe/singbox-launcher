package debuglog

import (
	"log"
)

// RunAndLog executes fn and logs label-prefixed error if it fails.
func RunAndLog(label string, fn func() error) {
	if err := fn(); err != nil {
		log.Printf("%s: %v", label, err)
	}
}
