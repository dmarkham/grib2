package grib2

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Section6 is the Bit-Map Section.
//
//	1-4    Section length
//	5      Number of section (6)
//	6      Bit-map indicator (Table 6.0)
//	         255 = A bit map does not apply to this product
//	         0   = A bit map applies to this product and is specified in this section
//	         1-254 = A bit map previously defined applies to this product
//	7-N    Bit-map (if indicator = 0)
type Section6 struct {
	Length    uint32
	Indicator uint8
	Bitmap    []byte // raw bitmap bytes (if indicator == 0)
}

// HasBitmap returns true if this section contains an actual bitmap.
func (s *Section6) HasBitmap() bool {
	return s.Indicator == 0
}

// ReadSection6 reads the Bit-Map Section.
func ReadSection6(r io.Reader) (Section6, error) {
	length, secNum, body, err := readSectionHeader(r)
	if err != nil {
		return Section6{}, fmt.Errorf("grib2: section 6: %w", err)
	}
	if secNum != 6 {
		return Section6{}, fmt.Errorf("grib2: expected section 6, got section %d", secNum)
	}

	if len(body) < 1 {
		return Section6{}, fmt.Errorf("grib2: section 6 body too short")
	}

	s := Section6{
		Length:    length,
		Indicator: body[0], // octet 6
	}

	if s.Indicator == 0 && len(body) > 1 {
		s.Bitmap = append([]byte(nil), body[1:]...)
	}

	return s, nil
}

// WriteSection6 writes the Bit-Map Section.
func WriteSection6(w io.Writer, s Section6) error {
	length := uint32(6 + len(s.Bitmap))
	buf := make([]byte, length)

	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = 6
	buf[5] = s.Indicator
	if len(s.Bitmap) > 0 {
		copy(buf[6:], s.Bitmap)
	}

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("grib2: writing section 6: %w", err)
	}
	return nil
}
