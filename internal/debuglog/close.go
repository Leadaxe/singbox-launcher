package debuglog

import (
	"io"
	"log"
)

// CloseWithLog closes the provided io.Closer and logs an error with context if closing fails.
// Safe to call with a nil closer.
func CloseWithLog(name string, c io.Closer) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		log.Printf("%s: failed to close: %v", name, err)
	}
}
