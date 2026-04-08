package grib2

import (
	"errors"
	"fmt"
	"io"
)

// Section8 is the End Section.
// It is always exactly 4 bytes: "7777" (ASCII).
const section8Magic = "7777"

var ErrInvalidEndSection = errors.New("grib2: invalid end section (expected 7777)")

// ReadSection8 reads and validates the 4-byte end marker.
func ReadSection8(r io.Reader) error {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return fmt.Errorf("grib2: reading section 8: %w", err)
	}
	if string(buf[:]) != section8Magic {
		return fmt.Errorf("%w: got %q", ErrInvalidEndSection, string(buf[:]))
	}
	return nil
}

// WriteSection8 writes the 4-byte end marker "7777".
func WriteSection8(w io.Writer) error {
	_, err := w.Write([]byte(section8Magic))
	if err != nil {
		return fmt.Errorf("grib2: writing section 8: %w", err)
	}
	return nil
}
