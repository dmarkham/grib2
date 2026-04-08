package grib2

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// Section1 is the Identification Section of a GRIB2 message.
//
// Octet layout (1-indexed, relative to section start):
//
//	1-4    Section length
//	5      Number of section (1)
//	6-7    Identification of originating/generating centre (Table 0)
//	8-9    Identification of originating/generating sub-centre (Table C)
//	10     GRIB master tables version number (Table 1.0)
//	11     Version number of GRIB local tables (Table 1.1)
//	12     Significance of reference time (Table 1.2)
//	13-14  Year
//	15     Month
//	16     Day
//	17     Hour
//	18     Minute
//	19     Second
//	20     Production status of processed data (Table 1.3)
//	21     Type of processed data (Table 1.4)
//	22-N   Reserved (optional, for future use)
type Section1 struct {
	Length                      uint32
	Centre                     uint16
	SubCentre                  uint16
	MasterTablesVersion        uint8
	LocalTablesVersion         uint8
	SignificanceOfReferenceTime uint8 // Table 1.2
	Year                       uint16
	Month                      uint8
	Day                        uint8
	Hour                       uint8
	Minute                     uint8
	Second                     uint8
	ProductionStatus           uint8 // Table 1.3
	TypeOfData                 uint8 // Table 1.4
}

const section1MinLength = 21

// ReferenceTime returns the reference time as a Go time.Time.
func (s *Section1) ReferenceTime() time.Time {
	return time.Date(int(s.Year), time.Month(s.Month), int(s.Day),
		int(s.Hour), int(s.Minute), int(s.Second), 0, time.UTC)
}

// ReadSection1 reads the Identification Section from r.
// The section header (5 bytes) is read first, then the body is parsed.
func ReadSection1(r io.Reader) (Section1, error) {
	length, secNum, body, err := readSectionHeader(r)
	if err != nil {
		return Section1{}, fmt.Errorf("grib2: section 1: %w", err)
	}
	if secNum != 1 {
		return Section1{}, fmt.Errorf("grib2: expected section 1, got section %d", secNum)
	}
	if length < section1MinLength {
		return Section1{}, fmt.Errorf("grib2: section 1 too short: %d bytes", length)
	}

	// body is offset from octet 6 (body[0] = octet 6)
	s := Section1{
		Length:                      length,
		Centre:                     readUint16(body, 0),  // octets 6-7
		SubCentre:                  readUint16(body, 2),  // octets 8-9
		MasterTablesVersion:        body[4],              // octet 10
		LocalTablesVersion:         body[5],              // octet 11
		SignificanceOfReferenceTime: body[6],             // octet 12
		Year:                       readUint16(body, 7),  // octets 13-14
		Month:                      body[9],              // octet 15
		Day:                        body[10],             // octet 16
		Hour:                       body[11],             // octet 17
		Minute:                     body[12],             // octet 18
		Second:                     body[13],             // octet 19
		ProductionStatus:           body[14],             // octet 20
		TypeOfData:                 body[15],             // octet 21
	}

	return s, nil
}

// WriteSection1 writes the Identification Section to w.
func WriteSection1(w io.Writer, s Section1) error {
	length := uint32(section1MinLength)
	buf := make([]byte, length)

	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = 1 // section number
	binary.BigEndian.PutUint16(buf[5:7], s.Centre)
	binary.BigEndian.PutUint16(buf[7:9], s.SubCentre)
	buf[9] = s.MasterTablesVersion
	buf[10] = s.LocalTablesVersion
	buf[11] = s.SignificanceOfReferenceTime
	binary.BigEndian.PutUint16(buf[12:14], s.Year)
	buf[14] = s.Month
	buf[15] = s.Day
	buf[16] = s.Hour
	buf[17] = s.Minute
	buf[18] = s.Second
	buf[19] = s.ProductionStatus
	buf[20] = s.TypeOfData

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("grib2: writing section 1: %w", err)
	}
	return nil
}
