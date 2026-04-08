package grib2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Section0 is the Indicator Section of a GRIB2 message.
// It is always exactly 16 bytes and identifies the start of a GRIB message.
//
// Octet layout (1-indexed per WMO spec):
//
//	1-4    "GRIB" (ASCII)
//	5-6    Reserved
//	7      Discipline (Table 0.0)
//	8      GRIB Edition Number (2)
//	9-16   Total length of GRIB message in octets
type Section0 struct {
	Reserved    [2]byte // Octets 5-6 (reserved by WMO, typically 0xFF 0xFF)
	Discipline  uint8   // WMO Table 0.0
	Edition     uint8   // Must be 2
	TotalLength uint64  // Total message length in bytes
}

const (
	section0Length = 16
	gribMagic      = "GRIB"
	grib2Edition   = 2
)

var (
	ErrNotGRIB      = errors.New("grib2: not a GRIB file (missing GRIB magic)")
	ErrNotEdition2  = errors.New("grib2: not GRIB edition 2")
	ErrTruncated    = errors.New("grib2: message truncated")
)

// ReadSection0 reads the 16-byte indicator section from r.
// It validates the GRIB magic number and edition.
func ReadSection0(r io.Reader) (Section0, error) {
	var buf [section0Length]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return Section0{}, ErrTruncated
		}
		return Section0{}, fmt.Errorf("grib2: reading section 0: %w", err)
	}

	if string(buf[0:4]) != gribMagic {
		return Section0{}, ErrNotGRIB
	}

	// bytes 5-6 are reserved (typically 0xFF 0xFF)
	var reserved [2]byte
	copy(reserved[:], buf[4:6])

	discipline := buf[6]
	edition := buf[7]

	if edition != grib2Edition {
		return Section0{}, fmt.Errorf("%w: got edition %d", ErrNotEdition2, edition)
	}

	totalLength := binary.BigEndian.Uint64(buf[8:16])

	return Section0{
		Reserved:    reserved,
		Discipline:  discipline,
		Edition:     edition,
		TotalLength: totalLength,
	}, nil
}

// WriteSection0 writes the 16-byte indicator section to w.
func WriteSection0(w io.Writer, s Section0) error {
	var buf [section0Length]byte
	copy(buf[0:4], gribMagic)
	buf[4] = s.Reserved[0] // reserved
	buf[5] = s.Reserved[1] // reserved
	buf[6] = s.Discipline
	buf[7] = grib2Edition
	binary.BigEndian.PutUint64(buf[8:16], s.TotalLength)

	_, err := w.Write(buf[:])
	if err != nil {
		return fmt.Errorf("grib2: writing section 0: %w", err)
	}
	return nil
}
