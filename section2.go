package grib2

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Section2 is the Local Use Section.
// It contains optional data defined by the originating centre.
//
// Octet layout:
//
//	1-4    Section length
//	5      Number of section (2)
//	6-N    Local use data
type Section2 struct {
	Length uint32
	Data   []byte // raw local-use bytes (may be empty)
}

// ReadSection2 reads the Local Use Section.
// Section 2 is optional — the caller should check the section number
// to determine whether to call this or skip to Section 3.
func ReadSection2(r io.Reader) (Section2, error) {
	length, secNum, body, err := readSectionHeader(r)
	if err != nil {
		return Section2{}, fmt.Errorf("grib2: section 2: %w", err)
	}
	if secNum != 2 {
		return Section2{}, fmt.Errorf("grib2: expected section 2, got section %d", secNum)
	}

	return Section2{
		Length: length,
		Data:   body,
	}, nil
}

// WriteSection2 writes the Local Use Section.
func WriteSection2(w io.Writer, s Section2) error {
	length := uint32(5 + len(s.Data))
	buf := make([]byte, length)
	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = 2
	copy(buf[5:], s.Data)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("grib2: writing section 2: %w", err)
	}
	return nil
}
