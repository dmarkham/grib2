package grib2

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Section5 is the Data Representation Section.
//
//	1-4    Section length
//	5      Number of section (5)
//	6-9    Number of data points where one or more values are
//	       specified in Section 7 when a bit map is present;
//	       total number of data points when a bit map is absent.
//	10-11  Data representation template number (Table 5.0)
//	12-N   Data representation template
type Section5 struct {
	Length         uint32
	NumberOfValues uint32
	TemplateNumber uint16 // Table 5.0
	Template       DataRepresentationTemplate
}

// DataRepresentationTemplate is the interface for data representation templates.
type DataRepresentationTemplate interface {
	TemplateNumber() uint16
}

// Template50 is Data Representation Template 5.0:
// Grid point data — simple packing.
//
//	12-15  Reference value (R) (IEEE 32-bit floating-point)
//	16-17  Binary scale factor (E) (signed)
//	18-19  Decimal scale factor (D) (signed)
//	20     Number of bits used for each packed value
//	21     Type of original field values (Table 5.1)
type Template50 struct {
	ReferenceValue    float32
	BinaryScaleFactor int16
	DecimalScaleFactor int16
	BitsPerValue      uint8
	TypeOfOriginalFieldValues uint8 // Table 5.1
}

func (Template50) TemplateNumber() uint16 { return 0 }

const template50Size = 10 // octets 12-21 = 10 bytes

// Template52 is Data Representation Template 5.2:
// Grid point data — complex packing.
// Extends Template 5.0 with group splitting parameters.
type Template52 struct {
	Template50                                   // octets 12-21
	GroupSplittingMethod                   uint8  // octet 22 (Table 5.4)
	MissingValueManagement                 uint8  // octet 23 (Table 5.5)
	PrimaryMissingValueSubstitute          uint32 // octets 24-27
	SecondaryMissingValueSubstitute        uint32 // octets 28-31
	NumberOfGroups                         uint32 // octets 32-35
	ReferenceForGroupWidths                uint8  // octet 36
	NumberOfBitsForGroupWidths             uint8  // octet 37
	ReferenceForGroupLengths               uint32 // octets 38-41
	LengthIncrementForGroupLengths         uint8  // octet 42
	TrueLengthOfLastGroup                  uint32 // octets 43-46
	NumberOfBitsForScaledGroupLengths      uint8  // octet 47
}

func (Template52) TemplateNumber() uint16 { return 2 }

const template52Size = 36 // octets 12-47 = 36 bytes

// Template53 is Data Representation Template 5.3:
// Grid point data — complex packing and spatial differencing.
// Extends Template 5.2 with spatial differencing parameters.
type Template53 struct {
	Template52                              // octets 12-47
	OrderOfSpatialDifferencing       uint8  // octet 48 (Table 5.6)
	NumberOfOctetsExtraDescriptors   uint8  // octet 49
}

func (Template53) TemplateNumber() uint16 { return 3 }

const template53Size = 38 // octets 12-49 = 38 bytes

// Template540 is Data Representation Template 5.40:
// Grid point data — JPEG 2000 code stream format.
// Extends Template 5.0 with compression parameters.
type Template540 struct {
	Template50
	TypeOfCompression      uint8 // octet 22 (Table 5.40): 0=lossless, 1=lossy
	TargetCompressionRatio uint8 // octet 23: 255=missing (lossless), or ratio for lossy
}

func (Template540) TemplateNumber() uint16 { return 40 }

const template540Size = 12 // octets 12-23 = 12 bytes

// Template542 is Data Representation Template 5.42:
// Grid point data — CCSDS recommended lossless compression.
// Extends Template 5.0 with CCSDS/AEC compression parameters.
//
//	12-21  Same as Template 5.0 (ref value, binary/decimal scale, bpv, type)
//	22     CCSDS compression options mask (ccsdsFlags)
//	23     Block size for CCSDS (ccsdsBlockSize)
//	24-25  Reference sample interval for CCSDS (ccsdsRsi)
type Template542 struct {
	Template50
	CcsdsFlags     uint8  // octet 22: CCSDS compression options mask
	CcsdsBlockSize uint8  // octet 23: block size
	CcsdsRsi       uint16 // octets 24-25: reference sample interval
}

func (Template542) TemplateNumber() uint16 { return 42 }

const template542Size = 14 // octets 12-25 = 14 bytes

// Template5200 is Data Representation Template 5.200:
// Grid point data -- run-length packing with level values.
//
//	Octet 12:      Number of bits used for each packed value (bitsPerValue)
//	Octets 13-14:  Maximum level value (maxLevelValue)
//	Octets 15-16:  Number of level values (numberOfLevelValues)
//	Octet 17:      Decimal scale factor
//	Octets 18-...: Level values (each 2 bytes, numberOfLevelValues entries)
type Template5200 struct {
	BitsPerValue         uint8
	MaxLevelValue        uint16
	NumberOfLevelValues  uint16
	DecimalScaleFactor   uint8
	LevelValues          []uint16
}

func (Template5200) TemplateNumber() uint16 { return 200 }

// RawDataRepTemplate stores an unparsed data representation template.
type RawDataRepTemplate struct {
	Number uint16
	Data   []byte
}

func (t *RawDataRepTemplate) TemplateNumber() uint16 { return t.Number }

// ReadSection5 reads the Data Representation Section.
func ReadSection5(r io.Reader) (Section5, error) {
	length, secNum, body, err := readSectionHeader(r)
	if err != nil {
		return Section5{}, fmt.Errorf("grib2: section 5: %w", err)
	}
	if secNum != 5 {
		return Section5{}, fmt.Errorf("grib2: expected section 5, got section %d", secNum)
	}

	if len(body) < 6 {
		return Section5{}, fmt.Errorf("grib2: section 5 body too short: %d bytes", len(body))
	}

	s := Section5{
		Length:         length,
		NumberOfValues: readUint32(body, 0), // octets 6-9
		TemplateNumber: readUint16(body, 4), // octets 10-11
	}

	templateBody := body[6:] // starts at octet 12

	switch s.TemplateNumber {
	case 0:
		tmpl, err := readTemplate50(templateBody)
		if err != nil {
			return Section5{}, err
		}
		s.Template = tmpl
	case 2:
		tmpl, err := readTemplate52(templateBody)
		if err != nil {
			return Section5{}, err
		}
		s.Template = tmpl
	case 3:
		tmpl, err := readTemplate53(templateBody)
		if err != nil {
			return Section5{}, err
		}
		s.Template = tmpl
	case 4:
		// IEEE floating point — just one byte for precision
		if len(templateBody) < 1 {
			return Section5{}, fmt.Errorf("grib2: template 5.4 too short")
		}
		s.Template = Template54{Precision: templateBody[0]}
	case 40:
		tmpl, err := readTemplate540(templateBody)
		if err != nil {
			return Section5{}, err
		}
		s.Template = tmpl
	case 41:
		// PNG packing uses the same template as simple packing (5.0)
		tmpl, err := readTemplate50(templateBody)
		if err != nil {
			return Section5{}, err
		}
		s.Template = tmpl
	case 42:
		tmpl, err := readTemplate542(templateBody)
		if err != nil {
			return Section5{}, err
		}
		s.Template = tmpl
	case 200:
		tmpl, err := readTemplate5200(templateBody)
		if err != nil {
			return Section5{}, err
		}
		s.Template = tmpl
	default:
		s.Template = &RawDataRepTemplate{
			Number: s.TemplateNumber,
			Data:   append([]byte(nil), templateBody...),
		}
	}

	return s, nil
}

func readTemplate50(body []byte) (Template50, error) {
	if len(body) < template50Size {
		return Template50{}, fmt.Errorf("grib2: template 5.0 too short: %d bytes, need %d", len(body), template50Size)
	}
	return Template50{
		ReferenceValue:            readFloat32IEEE(body, 0), // octets 12-15
		BinaryScaleFactor:         readSignMag16(body, 4),   // octets 16-17 (sign-magnitude)
		DecimalScaleFactor:        readSignMag16(body, 6),   // octets 18-19 (sign-magnitude)
		BitsPerValue:              body[8],                  // octet 20
		TypeOfOriginalFieldValues: body[9],                  // octet 21
	}, nil
}

func readTemplate540(body []byte) (Template540, error) {
	if len(body) < template540Size {
		return Template540{}, fmt.Errorf("grib2: template 5.40 too short: %d bytes, need %d", len(body), template540Size)
	}
	base, err := readTemplate50(body)
	if err != nil {
		return Template540{}, err
	}
	return Template540{
		Template50:             base,
		TypeOfCompression:      body[10], // octet 22
		TargetCompressionRatio: body[11], // octet 23
	}, nil
}

func writeTemplate540(t Template540) []byte {
	buf := make([]byte, template540Size)
	copy(buf[0:template50Size], writeTemplate50(t.Template50))
	buf[10] = t.TypeOfCompression
	buf[11] = t.TargetCompressionRatio
	return buf
}

func readTemplate542(body []byte) (Template542, error) {
	if len(body) < template542Size {
		return Template542{}, fmt.Errorf("grib2: template 5.42 too short: %d bytes, need %d", len(body), template542Size)
	}
	base, err := readTemplate50(body)
	if err != nil {
		return Template542{}, err
	}
	return Template542{
		Template50:     base,
		CcsdsFlags:     body[10],                                          // octet 22
		CcsdsBlockSize: body[11],                                          // octet 23
		CcsdsRsi:       uint16(body[12])<<8 | uint16(body[13]),            // octets 24-25
	}, nil
}

func writeTemplate542(t Template542) []byte {
	buf := make([]byte, template542Size)
	copy(buf[0:template50Size], writeTemplate50(t.Template50))
	buf[10] = t.CcsdsFlags
	buf[11] = t.CcsdsBlockSize
	buf[12] = byte(t.CcsdsRsi >> 8)
	buf[13] = byte(t.CcsdsRsi)
	return buf
}

func readTemplate52(body []byte) (Template52, error) {
	if len(body) < template52Size {
		return Template52{}, fmt.Errorf("grib2: template 5.2 too short: %d bytes, need %d", len(body), template52Size)
	}
	base, err := readTemplate50(body)
	if err != nil {
		return Template52{}, err
	}
	return Template52{
		Template50:                            base,
		GroupSplittingMethod:                  body[10],             // octet 22
		MissingValueManagement:                body[11],             // octet 23
		PrimaryMissingValueSubstitute:         readUint32(body, 12), // octets 24-27
		SecondaryMissingValueSubstitute:       readUint32(body, 16), // octets 28-31
		NumberOfGroups:                        readUint32(body, 20), // octets 32-35
		ReferenceForGroupWidths:               body[24],             // octet 36
		NumberOfBitsForGroupWidths:            body[25],             // octet 37
		ReferenceForGroupLengths:              readUint32(body, 26), // octets 38-41
		LengthIncrementForGroupLengths:        body[30],             // octet 42
		TrueLengthOfLastGroup:                readUint32(body, 31), // octets 43-46
		NumberOfBitsForScaledGroupLengths:     body[35],             // octet 47
	}, nil
}

func readTemplate53(body []byte) (Template53, error) {
	if len(body) < template53Size {
		return Template53{}, fmt.Errorf("grib2: template 5.3 too short: %d bytes, need %d", len(body), template53Size)
	}
	base, err := readTemplate52(body)
	if err != nil {
		return Template53{}, err
	}
	return Template53{
		Template52:                     base,
		OrderOfSpatialDifferencing:     body[36], // octet 48
		NumberOfOctetsExtraDescriptors: body[37], // octet 49
	}, nil
}

func writeTemplate52(t Template52) []byte {
	buf := make([]byte, template52Size)
	copy(buf[0:template50Size], writeTemplate50(t.Template50))
	buf[10] = t.GroupSplittingMethod
	buf[11] = t.MissingValueManagement
	binary.BigEndian.PutUint32(buf[12:16], t.PrimaryMissingValueSubstitute)
	binary.BigEndian.PutUint32(buf[16:20], t.SecondaryMissingValueSubstitute)
	binary.BigEndian.PutUint32(buf[20:24], t.NumberOfGroups)
	buf[24] = t.ReferenceForGroupWidths
	buf[25] = t.NumberOfBitsForGroupWidths
	binary.BigEndian.PutUint32(buf[26:30], t.ReferenceForGroupLengths)
	buf[30] = t.LengthIncrementForGroupLengths
	binary.BigEndian.PutUint32(buf[31:35], t.TrueLengthOfLastGroup)
	buf[35] = t.NumberOfBitsForScaledGroupLengths
	return buf
}

func writeTemplate53(t Template53) []byte {
	buf := make([]byte, template53Size)
	copy(buf[0:template52Size], writeTemplate52(t.Template52))
	buf[36] = t.OrderOfSpatialDifferencing
	buf[37] = t.NumberOfOctetsExtraDescriptors
	return buf
}

func readTemplate5200(body []byte) (Template5200, error) {
	// Minimum: 1 (bitsPerValue) + 2 (maxLevel) + 2 (numLevels) + 1 (decScale) = 6 bytes
	if len(body) < 6 {
		return Template5200{}, fmt.Errorf("grib2: template 5.200 too short: %d bytes, need at least 6", len(body))
	}
	t := Template5200{
		BitsPerValue:        body[0],             // octet 12
		MaxLevelValue:       readUint16(body, 1),  // octets 13-14
		NumberOfLevelValues: readUint16(body, 3),  // octets 15-16
		DecimalScaleFactor:  body[5],              // octet 17
	}
	n := int(t.NumberOfLevelValues)
	needed := 6 + n*2
	if len(body) < needed {
		return Template5200{}, fmt.Errorf("grib2: template 5.200 too short for %d level values: %d bytes, need %d", n, len(body), needed)
	}
	t.LevelValues = make([]uint16, n)
	for i := 0; i < n; i++ {
		t.LevelValues[i] = readUint16(body, 6+i*2) // octets 18+
	}
	return t, nil
}

func writeTemplate5200(t Template5200) []byte {
	n := int(t.NumberOfLevelValues)
	buf := make([]byte, 6+n*2)
	buf[0] = t.BitsPerValue
	binary.BigEndian.PutUint16(buf[1:3], t.MaxLevelValue)
	binary.BigEndian.PutUint16(buf[3:5], t.NumberOfLevelValues)
	buf[5] = t.DecimalScaleFactor
	for i := 0; i < n; i++ {
		binary.BigEndian.PutUint16(buf[6+i*2:8+i*2], t.LevelValues[i])
	}
	return buf
}

// WriteSection5 writes the Data Representation Section.
func WriteSection5(w io.Writer, s Section5) error {
	var templateBytes []byte

	switch tmpl := s.Template.(type) {
	case Template50:
		templateBytes = writeTemplate50(tmpl)
	case Template52:
		templateBytes = writeTemplate52(tmpl)
	case Template53:
		templateBytes = writeTemplate53(tmpl)
	case Template54:
		templateBytes = []byte{tmpl.Precision}
	case Template540:
		templateBytes = writeTemplate540(tmpl)
	case Template542:
		templateBytes = writeTemplate542(tmpl)
	case Template5200:
		templateBytes = writeTemplate5200(tmpl)
	case *RawDataRepTemplate:
		templateBytes = tmpl.Data
	default:
		return fmt.Errorf("grib2: unsupported data rep template type %T", s.Template)
	}

	length := uint32(11 + len(templateBytes))
	buf := make([]byte, length)

	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = 5
	binary.BigEndian.PutUint32(buf[5:9], s.NumberOfValues)
	binary.BigEndian.PutUint16(buf[9:11], s.TemplateNumber)
	copy(buf[11:], templateBytes)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("grib2: writing section 5: %w", err)
	}
	return nil
}

func writeTemplate50(t Template50) []byte {
	buf := make([]byte, template50Size)
	binary.BigEndian.PutUint32(buf[0:4], math.Float32bits(t.ReferenceValue))
	binary.BigEndian.PutUint16(buf[4:6], writeSignMag16(t.BinaryScaleFactor))
	binary.BigEndian.PutUint16(buf[6:8], writeSignMag16(t.DecimalScaleFactor))
	buf[8] = t.BitsPerValue
	buf[9] = t.TypeOfOriginalFieldValues
	return buf
}
