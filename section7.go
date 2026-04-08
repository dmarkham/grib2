package grib2

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Section7 is the Data Section.
//
//	1-4    Section length
//	5      Number of section (7)
//	6-N    Data in a format described by Data Template in Section 5
type Section7 struct {
	Length uint32
	Data   []byte // raw packed data bytes
}

// ReadSection7 reads the Data Section.
// The raw data is stored as-is; unpacking requires the Data Representation
// Template from Section 5.
func ReadSection7(r io.Reader) (Section7, error) {
	length, secNum, body, err := readSectionHeader(r)
	if err != nil {
		return Section7{}, fmt.Errorf("grib2: section 7: %w", err)
	}
	if secNum != 7 {
		return Section7{}, fmt.Errorf("grib2: expected section 7, got section %d", secNum)
	}

	return Section7{
		Length: length,
		Data:   body,
	}, nil
}

// WriteSection7 writes the Data Section.
func WriteSection7(w io.Writer, s Section7) error {
	length := uint32(5 + len(s.Data))
	buf := make([]byte, length)

	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = 7
	copy(buf[5:], s.Data)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("grib2: writing section 7: %w", err)
	}
	return nil
}
