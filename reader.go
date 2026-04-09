package grib2

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// readSectionHeader reads the 5-byte header common to sections 1-7:
//
//	Octets 1-4: Section length (uint32)
//	Octet 5:    Section number
//
// It returns the section length (including the 5 header bytes),
// the section number, and the raw body bytes (excluding the header).
func readSectionHeader(r io.Reader) (length uint32, sectionNum uint8, body []byte, err error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, 0, nil, fmt.Errorf("grib2: reading section header: %w", err)
	}

	length = binary.BigEndian.Uint32(hdr[0:4])
	sectionNum = hdr[4]

	if length < 5 {
		return 0, 0, nil, fmt.Errorf("grib2: section %d length %d too small", sectionNum, length)
	}

	// Guard against unreasonably large allocations from malformed input.
	// 256 MiB is generous for any legitimate GRIB2 section.
	const maxSectionSize = 256 * 1024 * 1024
	bodyLen := length - 5
	if bodyLen > maxSectionSize {
		return 0, 0, nil, fmt.Errorf("grib2: section %d length %d exceeds maximum allowed %d", sectionNum, length, maxSectionSize+5)
	}

	body = make([]byte, bodyLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, 0, nil, fmt.Errorf("grib2: reading section %d body: %w", sectionNum, err)
	}

	return length, sectionNum, body, nil
}

// readUint16 reads a big-endian uint16 from a byte slice at the given offset.
func readUint16(b []byte, offset int) uint16 {
	return binary.BigEndian.Uint16(b[offset : offset+2])
}

// readUint32 reads a big-endian uint32 from a byte slice at the given offset.
func readUint32(b []byte, offset int) uint32 {
	return binary.BigEndian.Uint32(b[offset : offset+4])
}

// readInt32 reads a big-endian signed int32 from a byte slice at the given offset.
// GRIB2 uses sign-magnitude for some fields (bit 1 = sign, bits 2-32 = magnitude),
// but most signed fields in the standard sections use two's complement via int32.
func readInt32(b []byte, offset int) int32 {
	return int32(binary.BigEndian.Uint32(b[offset : offset+4]))
}

// readInt16 reads a big-endian signed int16 from a byte slice at the given offset.
// Uses two's complement (standard Go interpretation).
func readInt16(b []byte, offset int) int16 {
	return int16(binary.BigEndian.Uint16(b[offset : offset+2]))
}

// readSignMag16 reads a big-endian sign-magnitude int16.
// GRIB2 uses sign-magnitude for scale factors: bit 1 = sign, bits 2-16 = magnitude.
func readSignMag16(b []byte, offset int) int16 {
	raw := binary.BigEndian.Uint16(b[offset : offset+2])
	magnitude := int16(raw & 0x7FFF)
	if raw&0x8000 != 0 {
		return -magnitude
	}
	return magnitude
}

// writeSignMag16 encodes an int16 as sign-magnitude big-endian.
func writeSignMag16(val int16) uint16 {
	if val < 0 {
		return 0x8000 | uint16(-val)
	}
	return uint16(val)
}

// readSignMag32 reads a big-endian sign-magnitude int32.
// GRIB2 uses sign-magnitude for latitudes and longitudes:
// bit 1 = sign (1 = negative), bits 2-32 = magnitude.
func readSignMag32(b []byte, offset int) int32 {
	raw := binary.BigEndian.Uint32(b[offset : offset+4])
	magnitude := int32(raw & 0x7FFFFFFF)
	if raw&0x80000000 != 0 {
		return -magnitude
	}
	return magnitude
}

// writeSignMag32 encodes an int32 as sign-magnitude big-endian.
func writeSignMag32(val int32) uint32 {
	if val < 0 {
		return 0x80000000 | uint32(-val)
	}
	return uint32(val)
}

// readSignMag8 reads a sign-magnitude int8.
// Bit 7 = sign (1 = negative), bits 0-6 = magnitude.
func readSignMag8(b byte) int8 {
	if b&0x80 != 0 {
		return -int8(b & 0x7F)
	}
	return int8(b)
}

// writeSignMag8 encodes an int8 as sign-magnitude.
func writeSignMag8(val int8) byte {
	if val < 0 {
		return 0x80 | byte(-val)
	}
	return byte(val)
}

// readFloat32IEEE reads a big-endian IEEE 754 float32.
func readFloat32IEEE(b []byte, offset int) float32 {
	bits := binary.BigEndian.Uint32(b[offset : offset+4])
	return math.Float32frombits(bits)
}
