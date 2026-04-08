package grib2

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Section3 is the Grid Definition Section.
//
// Common header (octets 1-14):
//
//	1-4    Section length
//	5      Number of section (3)
//	6      Source of grid definition (Table 3.0)
//	7-10   Number of data points
//	11     Number of octets for optional list of numbers
//	12     Interpretation of list of numbers (Table 3.11)
//	13-14  Grid definition template number (Table 3.1)
//	15-N   Grid definition template (varies by template number)
type Section3 struct {
	Length                   uint32
	Source                   uint8  // Table 3.0
	NumberOfDataPoints       uint32
	OctetsForNumberOfPoints  uint8
	InterpretationOfPoints   uint8  // Table 3.11
	TemplateNumber           uint16 // Table 3.1
	Template                 GridDefinitionTemplate
	OptionalPointNumbers     []byte // optional list appended after template
}

// GridDefinitionTemplate is the interface for grid definition templates.
type GridDefinitionTemplate interface {
	TemplateNumber() uint16
}

// Template30 is Grid Definition Template 3.0:
// Latitude/Longitude (equidistant cylindrical, or Plate Carree).
//
// All latitudes/longitudes are in units of 10^-6 degrees.
type Template30 struct {
	ShapeOfEarth                       uint8  // Table 3.2
	ScaleFactorOfRadiusOfSphericalEarth uint8
	ScaledValueOfRadiusOfSphericalEarth uint32
	ScaleFactorOfEarthMajorAxis        uint8
	ScaledValueOfEarthMajorAxis        uint32
	ScaleFactorOfEarthMinorAxis        uint8
	ScaledValueOfEarthMinorAxis        uint32
	Ni                                 uint32 // number of points along a parallel
	Nj                                 uint32 // number of points along a meridian
	BasicAngle                         uint32
	SubdivisionsOfBasicAngle           uint32
	LatitudeOfFirstGridPoint           int32  // microdegrees
	LongitudeOfFirstGridPoint          int32  // microdegrees
	ResolutionAndComponentFlags        uint8  // Flag table 3.3
	LatitudeOfLastGridPoint            int32  // microdegrees
	LongitudeOfLastGridPoint           int32  // microdegrees
	IDirectionIncrement                uint32 // microdegrees
	JDirectionIncrement                uint32 // microdegrees
	ScanningMode                       uint8  // Flag table 3.4
}

func (Template30) TemplateNumber() uint16 { return 0 }

// Template31 is Grid Definition Template 3.1:
// Rotated Latitude/Longitude (rotated equidistant cylindrical, or rotated Plate Carree).
//
// Identical to Template 3.0 for octets 15-72, with three additional fields
// (octets 73-84) describing the rotation.
// All latitudes/longitudes are in units of 10^-6 degrees (sign-magnitude encoding).
type Template31 struct {
	ShapeOfEarth                        uint8  // Table 3.2
	ScaleFactorOfRadiusOfSphericalEarth uint8
	ScaledValueOfRadiusOfSphericalEarth uint32
	ScaleFactorOfEarthMajorAxis         uint8
	ScaledValueOfEarthMajorAxis         uint32
	ScaleFactorOfEarthMinorAxis         uint8
	ScaledValueOfEarthMinorAxis         uint32
	Ni                                  uint32 // number of points along a parallel
	Nj                                  uint32 // number of points along a meridian
	BasicAngle                          uint32
	SubdivisionsOfBasicAngle            uint32
	LatitudeOfFirstGridPoint            int32 // microdegrees (sign-magnitude)
	LongitudeOfFirstGridPoint           int32 // microdegrees (sign-magnitude)
	ResolutionAndComponentFlags         uint8 // Flag table 3.3
	LatitudeOfLastGridPoint             int32 // microdegrees (sign-magnitude)
	LongitudeOfLastGridPoint            int32 // microdegrees (sign-magnitude)
	IDirectionIncrement                 uint32 // microdegrees
	JDirectionIncrement                 uint32 // microdegrees
	ScanningMode                        uint8  // Flag table 3.4
	LatitudeOfSouthernPole              int32  // microdegrees (sign-magnitude)
	LongitudeOfSouthernPole             int32  // microdegrees (sign-magnitude)
	AngleOfRotation                     uint32 // angle of rotation in microdegrees
}

func (Template31) TemplateNumber() uint16 { return 1 }

// Template310 is Grid Definition Template 3.10: Mercator.
//
// All latitudes/longitudes are in units of 10^-6 degrees (sign-magnitude encoding).
// Di and Dj are in units of 10^-3 metres (millimetres).
type Template310 struct {
	ShapeOfEarth                        uint8  // Table 3.2
	ScaleFactorOfRadiusOfSphericalEarth uint8
	ScaledValueOfRadiusOfSphericalEarth uint32
	ScaleFactorOfEarthMajorAxis         uint8
	ScaledValueOfEarthMajorAxis         uint32
	ScaleFactorOfEarthMinorAxis         uint8
	ScaledValueOfEarthMinorAxis         uint32
	Ni                                  uint32 // number of points along a parallel
	Nj                                  uint32 // number of points along a meridian
	LatitudeOfFirstGridPoint            int32  // microdegrees (sign-magnitude)
	LongitudeOfFirstGridPoint           int32  // microdegrees (sign-magnitude)
	ResolutionAndComponentFlags         uint8  // Flag table 3.3
	LaD                                 int32  // latitude where Dx and Dy are specified (sign-magnitude)
	LatitudeOfLastGridPoint             int32  // microdegrees (sign-magnitude)
	LongitudeOfLastGridPoint            int32  // microdegrees (sign-magnitude)
	ScanningMode                        uint8  // Flag table 3.4
	OrientationOfTheGrid                uint32 // orientation angle in microdegrees
	Di                                  uint32 // longitudinal direction grid length in millimetres
	Dj                                  uint32 // latitudinal direction grid length in millimetres
}

func (Template310) TemplateNumber() uint16 { return 10 }

// Template320 is Grid Definition Template 3.20: Polar Stereographic.
//
// All latitudes/longitudes are in units of 10^-6 degrees (sign-magnitude encoding).
// Per the WMO GRIB2 specification, all lat/lon fields use sign-magnitude
// encoding (MSB=1 means negative). This matches eccodes' unsigned[4]
// declaration — eccodes handles the sign-magnitude conversion internally.
// Longitudes can range from -180 to +360 depending on convention.
// Dx and Dy are in units of 10^-3 metres (millimetres).
type Template320 struct {
	ShapeOfEarth                        uint8  // Table 3.2
	ScaleFactorOfRadiusOfSphericalEarth uint8
	ScaledValueOfRadiusOfSphericalEarth uint32
	ScaleFactorOfEarthMajorAxis         uint8
	ScaledValueOfEarthMajorAxis         uint32
	ScaleFactorOfEarthMinorAxis         uint8
	ScaledValueOfEarthMinorAxis         uint32
	Nx                                  uint32 // number of points along the x-axis
	Ny                                  uint32 // number of points along the y-axis
	LatitudeOfFirstGridPoint            int32  // microdegrees (sign-magnitude)
	LongitudeOfFirstGridPoint           int32  // microdegrees (sign-magnitude)
	ResolutionAndComponentFlags         uint8  // Flag table 3.3
	LaD                                 int32  // latitude where Dx and Dy are specified (sign-magnitude)
	OrientationOfTheGrid                int32  // orientation of the grid, microdegrees (sign-magnitude)
	Dx                                  uint32 // x-direction grid length in millimetres
	Dy                                  uint32 // y-direction grid length in millimetres
	ProjectionCentreFlag                uint8  // Flag table 3.5
	ScanningMode                        uint8  // Flag table 3.4
}

func (Template320) TemplateNumber() uint16 { return 20 }

// Template330 is Grid Definition Template 3.30:
// Lambert Conformal (can be secant or tangent, conical or bipolar).
//
// All latitudes/longitudes are in units of 10^-6 degrees.
// Per the WMO GRIB2 specification, all lat/lon fields use sign-magnitude
// encoding (MSB=1 means negative). This matches eccodes' unsigned[4]
// declaration — eccodes handles the sign-magnitude conversion internally.
// Longitudes can range from -180 to +360 depending on convention.
// Dx and Dy are in units of 10^-3 metres (millimetres).
type Template330 struct {
	ShapeOfEarth                        uint8  // Table 3.2
	ScaleFactorOfRadiusOfSphericalEarth uint8
	ScaledValueOfRadiusOfSphericalEarth uint32
	ScaleFactorOfEarthMajorAxis         uint8
	ScaledValueOfEarthMajorAxis         uint32
	ScaleFactorOfEarthMinorAxis         uint8
	ScaledValueOfEarthMinorAxis         uint32
	Nx                                  uint32 // number of points along the X-axis
	Ny                                  uint32 // number of points along the Y-axis
	LatitudeOfFirstGridPoint            int32  // microdegrees
	LongitudeOfFirstGridPoint           int32  // microdegrees (sign-magnitude)
	ResolutionAndComponentFlags         uint8  // Flag table 3.3
	LaD                                 int32  // latitude where Dx and Dy are specified, microdegrees
	LoV                                 int32  // longitude of meridian parallel to Y-axis, microdegrees (sign-magnitude)
	Dx                                  uint32 // X-direction grid length in millimetres
	Dy                                  uint32 // Y-direction grid length in millimetres
	ProjectionCentreFlag                uint8  // Flag table 3.5
	ScanningMode                        uint8  // Flag table 3.4
	Latin1                              int32  // first standard parallel, microdegrees
	Latin2                              int32  // second standard parallel, microdegrees
	LatitudeOfSouthernPole              int32  // microdegrees
	LongitudeOfSouthernPole             int32  // microdegrees (sign-magnitude)
}

func (Template330) TemplateNumber() uint16 { return 30 }

// Template340 is Grid Definition Template 3.40:
// Gaussian Latitude/Longitude.
//
// Identical layout to Template 3.0 except octets 68-71 hold N (number
// of parallels between a pole and the equator) instead of
// jDirectionIncrement.
//
// For reduced Gaussian grids, Ni is set to 0xFFFFFFFF (MISSING) and an
// optional PL array (number of points per latitude row) is appended
// after the template in Section3.OptionalPointNumbers.
type Template340 struct {
	ShapeOfEarth                        uint8  // Table 3.2
	ScaleFactorOfRadiusOfSphericalEarth uint8
	ScaledValueOfRadiusOfSphericalEarth uint32
	ScaleFactorOfEarthMajorAxis         uint8
	ScaledValueOfEarthMajorAxis         uint32
	ScaleFactorOfEarthMinorAxis         uint8
	ScaledValueOfEarthMinorAxis         uint32
	Ni                                  uint32 // number of points along a parallel (MISSING for reduced)
	Nj                                  uint32 // number of points along a meridian
	BasicAngle                          uint32
	SubdivisionsOfBasicAngle            uint32
	LatitudeOfFirstGridPoint            int32  // microdegrees
	LongitudeOfFirstGridPoint           int32  // microdegrees
	ResolutionAndComponentFlags         uint8  // Flag table 3.3
	LatitudeOfLastGridPoint             int32  // microdegrees
	LongitudeOfLastGridPoint            int32  // microdegrees
	IDirectionIncrement                 uint32 // microdegrees (may be MISSING for reduced)
	N                                   uint32 // number of parallels between pole and equator
	ScanningMode                        uint8  // Flag table 3.4
}

func (Template340) TemplateNumber() uint16 { return 40 }

// Template390 is Grid Definition Template 3.90:
// Space view perspective or orthographic.
// Used by geostationary satellites (GOES-R, Himawari, MSG).
//
// All latitudes/longitudes are in units of 10^-6 degrees (sign-magnitude encoding).
// Xp, Yp are in units of 10^-3 grid lengths.
// Nr is in units of 10^-6 of the Earth's equatorial radius.
type Template390 struct {
	ShapeOfEarth                        uint8  // Table 3.2
	ScaleFactorOfRadiusOfSphericalEarth uint8
	ScaledValueOfRadiusOfSphericalEarth uint32
	ScaleFactorOfEarthMajorAxis         uint8
	ScaledValueOfEarthMajorAxis         uint32
	ScaleFactorOfEarthMinorAxis         uint8
	ScaledValueOfEarthMinorAxis         uint32
	Nx                                  uint32 // number of points along X-axis
	Ny                                  uint32 // number of points along Y-axis
	LatitudeOfSubSatellitePoint         int32  // microdegrees (sign-magnitude)
	LongitudeOfSubSatellitePoint        int32  // microdegrees (sign-magnitude)
	ResolutionAndComponentFlags         uint8  // Flag table 3.3
	Dx                                  uint32 // apparent diameter of Earth in grid lengths, X-direction
	Dy                                  uint32 // apparent diameter of Earth in grid lengths, Y-direction
	Xp                                  uint32 // X-coordinate of sub-satellite point (10^-3 grid lengths)
	Yp                                  uint32 // Y-coordinate of sub-satellite point (10^-3 grid lengths)
	ScanningMode                        uint8  // Flag table 3.4
	OrientationOfTheGrid                int32  // angle in microdegrees (sign-magnitude)
	Nr                                  uint32 // altitude of camera from Earth centre (10^-6 Earth radii)
	Xo                                  uint32 // X-coordinate of origin of sector image
	Yo                                  uint32 // Y-coordinate of origin of sector image
}

func (Template390) TemplateNumber() uint16 { return 90 }

// ReadSection3 reads the Grid Definition Section.
func ReadSection3(r io.Reader) (Section3, error) {
	length, secNum, body, err := readSectionHeader(r)
	if err != nil {
		return Section3{}, fmt.Errorf("grib2: section 3: %w", err)
	}
	if secNum != 3 {
		return Section3{}, fmt.Errorf("grib2: expected section 3, got section %d", secNum)
	}

	if len(body) < 9 {
		return Section3{}, fmt.Errorf("grib2: section 3 body too short: %d bytes", len(body))
	}

	s := Section3{
		Length:                  length,
		Source:                  body[0],              // octet 6
		NumberOfDataPoints:      readUint32(body, 1),  // octets 7-10
		OctetsForNumberOfPoints: body[5],              // octet 11
		InterpretationOfPoints:  body[6],              // octet 12
		TemplateNumber:          readUint16(body, 7),  // octets 13-14
	}

	templateBody := body[9:] // starts at octet 15

	switch s.TemplateNumber {
	case 0:
		tmpl, err := readTemplate30(templateBody)
		if err != nil {
			return Section3{}, err
		}
		s.Template = tmpl
	case 1:
		tmpl, err := readTemplate31(templateBody)
		if err != nil {
			return Section3{}, err
		}
		s.Template = tmpl
	case 10:
		tmpl, err := readTemplate310(templateBody)
		if err != nil {
			return Section3{}, err
		}
		s.Template = tmpl
	case 20:
		tmpl, err := readTemplate320(templateBody)
		if err != nil {
			return Section3{}, err
		}
		s.Template = tmpl
	case 30:
		tmpl, err := readTemplate330(templateBody)
		if err != nil {
			return Section3{}, err
		}
		s.Template = tmpl
	case 40:
		tmpl, optPL, err := readTemplate340(templateBody, s.OctetsForNumberOfPoints)
		if err != nil {
			return Section3{}, err
		}
		s.Template = tmpl
		if len(optPL) > 0 {
			s.OptionalPointNumbers = optPL
		}
	case 90:
		tmpl, err := readTemplate390(templateBody)
		if err != nil {
			return Section3{}, err
		}
		s.Template = tmpl
	default:
		// Store as raw template for unsupported template numbers
		s.Template = &RawGridTemplate{
			Number: s.TemplateNumber,
			Data:   append([]byte(nil), templateBody...),
		}
	}

	return s, nil
}

// RawGridTemplate stores an unparsed grid definition template.
type RawGridTemplate struct {
	Number uint16
	Data   []byte
}

func (t *RawGridTemplate) TemplateNumber() uint16 { return t.Number }

const template30Size = 58 // octets 15-72 = 58 bytes

func readTemplate30(body []byte) (Template30, error) {
	if len(body) < template30Size {
		return Template30{}, fmt.Errorf("grib2: template 3.0 too short: %d bytes, need %d", len(body), template30Size)
	}

	return Template30{
		ShapeOfEarth:                       body[0],                  // octet 15
		ScaleFactorOfRadiusOfSphericalEarth: body[1],                 // octet 16
		ScaledValueOfRadiusOfSphericalEarth: readUint32(body, 2),     // octets 17-20
		ScaleFactorOfEarthMajorAxis:        body[6],                  // octet 21
		ScaledValueOfEarthMajorAxis:        readUint32(body, 7),      // octets 22-25
		ScaleFactorOfEarthMinorAxis:        body[11],                 // octet 26
		ScaledValueOfEarthMinorAxis:        readUint32(body, 12),     // octets 27-30
		Ni:                                 readUint32(body, 16),     // octets 31-34
		Nj:                                 readUint32(body, 20),     // octets 35-38
		BasicAngle:                         readUint32(body, 24),     // octets 39-42
		SubdivisionsOfBasicAngle:           readUint32(body, 28),     // octets 43-46
		LatitudeOfFirstGridPoint:           readSignMag32(body, 32),  // octets 47-50
		LongitudeOfFirstGridPoint:          readSignMag32(body, 36),  // octets 51-54
		ResolutionAndComponentFlags:        body[40],                 // octet 55
		LatitudeOfLastGridPoint:            readSignMag32(body, 41),  // octets 56-59
		LongitudeOfLastGridPoint:           readSignMag32(body, 45),  // octets 60-63
		IDirectionIncrement:                readUint32(body, 49),     // octets 64-67
		JDirectionIncrement:                readUint32(body, 53),     // octets 68-71
		ScanningMode:                       body[57],                 // octet 72
	}, nil
}

// WriteSection3 writes the Grid Definition Section.
func WriteSection3(w io.Writer, s Section3) error {
	var templateBytes []byte

	switch tmpl := s.Template.(type) {
	case Template30:
		templateBytes = writeTemplate30(tmpl)
	case Template31:
		templateBytes = writeTemplate31(tmpl)
	case Template310:
		templateBytes = writeTemplate310(tmpl)
	case Template320:
		templateBytes = writeTemplate320(tmpl)
	case Template330:
		templateBytes = writeTemplate330(tmpl)
	case Template340:
		templateBytes = writeTemplate340(tmpl)
	case Template390:
		templateBytes = writeTemplate390(tmpl)
	case *RawGridTemplate:
		templateBytes = tmpl.Data
	default:
		return fmt.Errorf("grib2: unsupported grid template type %T", s.Template)
	}

	length := uint32(14 + len(templateBytes) + len(s.OptionalPointNumbers))
	buf := make([]byte, length)

	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = 3
	buf[5] = s.Source
	binary.BigEndian.PutUint32(buf[6:10], s.NumberOfDataPoints)
	buf[10] = s.OctetsForNumberOfPoints
	buf[11] = s.InterpretationOfPoints
	binary.BigEndian.PutUint16(buf[12:14], s.TemplateNumber)
	copy(buf[14:], templateBytes)
	if len(s.OptionalPointNumbers) > 0 {
		copy(buf[14+len(templateBytes):], s.OptionalPointNumbers)
	}

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("grib2: writing section 3: %w", err)
	}
	return nil
}

func writeTemplate30(t Template30) []byte {
	buf := make([]byte, template30Size)
	buf[0] = t.ShapeOfEarth
	buf[1] = t.ScaleFactorOfRadiusOfSphericalEarth
	binary.BigEndian.PutUint32(buf[2:6], t.ScaledValueOfRadiusOfSphericalEarth)
	buf[6] = t.ScaleFactorOfEarthMajorAxis
	binary.BigEndian.PutUint32(buf[7:11], t.ScaledValueOfEarthMajorAxis)
	buf[11] = t.ScaleFactorOfEarthMinorAxis
	binary.BigEndian.PutUint32(buf[12:16], t.ScaledValueOfEarthMinorAxis)
	binary.BigEndian.PutUint32(buf[16:20], t.Ni)
	binary.BigEndian.PutUint32(buf[20:24], t.Nj)
	binary.BigEndian.PutUint32(buf[24:28], t.BasicAngle)
	binary.BigEndian.PutUint32(buf[28:32], t.SubdivisionsOfBasicAngle)
	binary.BigEndian.PutUint32(buf[32:36], writeSignMag32(t.LatitudeOfFirstGridPoint))
	binary.BigEndian.PutUint32(buf[36:40], writeSignMag32(t.LongitudeOfFirstGridPoint))
	buf[40] = t.ResolutionAndComponentFlags
	binary.BigEndian.PutUint32(buf[41:45], writeSignMag32(t.LatitudeOfLastGridPoint))
	binary.BigEndian.PutUint32(buf[45:49], writeSignMag32(t.LongitudeOfLastGridPoint))
	binary.BigEndian.PutUint32(buf[49:53], t.IDirectionIncrement)
	binary.BigEndian.PutUint32(buf[53:57], t.JDirectionIncrement)
	buf[57] = t.ScanningMode
	return buf
}

// Template 3.1 - Rotated Latitude/Longitude
// Template body is octets 15-84 = 70 bytes (58 bytes from 3.0 + 12 rotation bytes).
const template31Size = 70

func readTemplate31(body []byte) (Template31, error) {
	if len(body) < template31Size {
		return Template31{}, fmt.Errorf("grib2: template 3.1 too short: %d bytes, need %d", len(body), template31Size)
	}

	return Template31{
		ShapeOfEarth:                        body[0],                  // octet 15
		ScaleFactorOfRadiusOfSphericalEarth: body[1],                  // octet 16
		ScaledValueOfRadiusOfSphericalEarth: readUint32(body, 2),      // octets 17-20
		ScaleFactorOfEarthMajorAxis:         body[6],                  // octet 21
		ScaledValueOfEarthMajorAxis:         readUint32(body, 7),      // octets 22-25
		ScaleFactorOfEarthMinorAxis:         body[11],                 // octet 26
		ScaledValueOfEarthMinorAxis:         readUint32(body, 12),     // octets 27-30
		Ni:                                  readUint32(body, 16),     // octets 31-34
		Nj:                                  readUint32(body, 20),     // octets 35-38
		BasicAngle:                          readUint32(body, 24),     // octets 39-42
		SubdivisionsOfBasicAngle:            readUint32(body, 28),     // octets 43-46
		LatitudeOfFirstGridPoint:            readSignMag32(body, 32),  // octets 47-50
		LongitudeOfFirstGridPoint:           readSignMag32(body, 36),  // octets 51-54
		ResolutionAndComponentFlags:         body[40],                 // octet 55
		LatitudeOfLastGridPoint:             readSignMag32(body, 41),  // octets 56-59
		LongitudeOfLastGridPoint:            readSignMag32(body, 45),  // octets 60-63
		IDirectionIncrement:                 readUint32(body, 49),     // octets 64-67
		JDirectionIncrement:                 readUint32(body, 53),     // octets 68-71
		ScanningMode:                        body[57],                 // octet 72
		LatitudeOfSouthernPole:              readSignMag32(body, 58),  // octets 73-76
		LongitudeOfSouthernPole:             readSignMag32(body, 62),  // octets 77-80
		AngleOfRotation:                     readUint32(body, 66),     // octets 81-84
	}, nil
}

func writeTemplate31(t Template31) []byte {
	buf := make([]byte, template31Size)
	buf[0] = t.ShapeOfEarth
	buf[1] = t.ScaleFactorOfRadiusOfSphericalEarth
	binary.BigEndian.PutUint32(buf[2:6], t.ScaledValueOfRadiusOfSphericalEarth)
	buf[6] = t.ScaleFactorOfEarthMajorAxis
	binary.BigEndian.PutUint32(buf[7:11], t.ScaledValueOfEarthMajorAxis)
	buf[11] = t.ScaleFactorOfEarthMinorAxis
	binary.BigEndian.PutUint32(buf[12:16], t.ScaledValueOfEarthMinorAxis)
	binary.BigEndian.PutUint32(buf[16:20], t.Ni)
	binary.BigEndian.PutUint32(buf[20:24], t.Nj)
	binary.BigEndian.PutUint32(buf[24:28], t.BasicAngle)
	binary.BigEndian.PutUint32(buf[28:32], t.SubdivisionsOfBasicAngle)
	binary.BigEndian.PutUint32(buf[32:36], writeSignMag32(t.LatitudeOfFirstGridPoint))
	binary.BigEndian.PutUint32(buf[36:40], writeSignMag32(t.LongitudeOfFirstGridPoint))
	buf[40] = t.ResolutionAndComponentFlags
	binary.BigEndian.PutUint32(buf[41:45], writeSignMag32(t.LatitudeOfLastGridPoint))
	binary.BigEndian.PutUint32(buf[45:49], writeSignMag32(t.LongitudeOfLastGridPoint))
	binary.BigEndian.PutUint32(buf[49:53], t.IDirectionIncrement)
	binary.BigEndian.PutUint32(buf[53:57], t.JDirectionIncrement)
	buf[57] = t.ScanningMode
	binary.BigEndian.PutUint32(buf[58:62], writeSignMag32(t.LatitudeOfSouthernPole))
	binary.BigEndian.PutUint32(buf[62:66], writeSignMag32(t.LongitudeOfSouthernPole))
	binary.BigEndian.PutUint32(buf[66:70], t.AngleOfRotation)
	return buf
}

// Template 3.10 - Mercator
// Template body is octets 15-72 = 58 bytes.
const template310Size = 58

func readTemplate310(body []byte) (Template310, error) {
	if len(body) < template310Size {
		return Template310{}, fmt.Errorf("grib2: template 3.10 too short: %d bytes, need %d", len(body), template310Size)
	}

	return Template310{
		ShapeOfEarth:                        body[0],                  // octet 15
		ScaleFactorOfRadiusOfSphericalEarth: body[1],                  // octet 16
		ScaledValueOfRadiusOfSphericalEarth: readUint32(body, 2),      // octets 17-20
		ScaleFactorOfEarthMajorAxis:         body[6],                  // octet 21
		ScaledValueOfEarthMajorAxis:         readUint32(body, 7),      // octets 22-25
		ScaleFactorOfEarthMinorAxis:         body[11],                 // octet 26
		ScaledValueOfEarthMinorAxis:         readUint32(body, 12),     // octets 27-30
		Ni:                                  readUint32(body, 16),     // octets 31-34
		Nj:                                  readUint32(body, 20),     // octets 35-38
		LatitudeOfFirstGridPoint:            readSignMag32(body, 24),  // octets 39-42
		LongitudeOfFirstGridPoint:           readSignMag32(body, 28),  // octets 43-46
		ResolutionAndComponentFlags:         body[32],                 // octet 47
		LaD:                                 readSignMag32(body, 33),  // octets 48-51
		LatitudeOfLastGridPoint:             readSignMag32(body, 37),  // octets 52-55
		LongitudeOfLastGridPoint:            readSignMag32(body, 41),  // octets 56-59
		ScanningMode:                        body[45],                 // octet 60
		OrientationOfTheGrid:                readUint32(body, 46),     // octets 61-64
		Di:                                  readUint32(body, 50),     // octets 65-68
		Dj:                                  readUint32(body, 54),     // octets 69-72
	}, nil
}

func writeTemplate310(t Template310) []byte {
	buf := make([]byte, template310Size)
	buf[0] = t.ShapeOfEarth
	buf[1] = t.ScaleFactorOfRadiusOfSphericalEarth
	binary.BigEndian.PutUint32(buf[2:6], t.ScaledValueOfRadiusOfSphericalEarth)
	buf[6] = t.ScaleFactorOfEarthMajorAxis
	binary.BigEndian.PutUint32(buf[7:11], t.ScaledValueOfEarthMajorAxis)
	buf[11] = t.ScaleFactorOfEarthMinorAxis
	binary.BigEndian.PutUint32(buf[12:16], t.ScaledValueOfEarthMinorAxis)
	binary.BigEndian.PutUint32(buf[16:20], t.Ni)
	binary.BigEndian.PutUint32(buf[20:24], t.Nj)
	binary.BigEndian.PutUint32(buf[24:28], writeSignMag32(t.LatitudeOfFirstGridPoint))
	binary.BigEndian.PutUint32(buf[28:32], writeSignMag32(t.LongitudeOfFirstGridPoint))
	buf[32] = t.ResolutionAndComponentFlags
	binary.BigEndian.PutUint32(buf[33:37], writeSignMag32(t.LaD))
	binary.BigEndian.PutUint32(buf[37:41], writeSignMag32(t.LatitudeOfLastGridPoint))
	binary.BigEndian.PutUint32(buf[41:45], writeSignMag32(t.LongitudeOfLastGridPoint))
	buf[45] = t.ScanningMode
	binary.BigEndian.PutUint32(buf[46:50], t.OrientationOfTheGrid)
	binary.BigEndian.PutUint32(buf[50:54], t.Di)
	binary.BigEndian.PutUint32(buf[54:58], t.Dj)
	return buf
}

// Template 3.20 - Polar Stereographic
// Template body is octets 15-65 = 51 bytes.
const template320Size = 51

func readTemplate320(body []byte) (Template320, error) {
	if len(body) < template320Size {
		return Template320{}, fmt.Errorf("grib2: template 3.20 too short: %d bytes, need %d", len(body), template320Size)
	}

	return Template320{
		ShapeOfEarth:                        body[0],                  // octet 15
		ScaleFactorOfRadiusOfSphericalEarth: body[1],                  // octet 16
		ScaledValueOfRadiusOfSphericalEarth: readUint32(body, 2),      // octets 17-20
		ScaleFactorOfEarthMajorAxis:         body[6],                  // octet 21
		ScaledValueOfEarthMajorAxis:         readUint32(body, 7),      // octets 22-25
		ScaleFactorOfEarthMinorAxis:         body[11],                 // octet 26
		ScaledValueOfEarthMinorAxis:         readUint32(body, 12),     // octets 27-30
		Nx:                                  readUint32(body, 16),     // octets 31-34
		Ny:                                  readUint32(body, 20),     // octets 35-38
		LatitudeOfFirstGridPoint:            readSignMag32(body, 24),  // octets 39-42
		LongitudeOfFirstGridPoint:           readSignMag32(body, 28),  // octets 43-46
		ResolutionAndComponentFlags:         body[32],                 // octet 47
		LaD:                                 readSignMag32(body, 33),  // octets 48-51
		OrientationOfTheGrid:                readSignMag32(body, 37),  // octets 52-55
		Dx:                                  readUint32(body, 41),     // octets 56-59
		Dy:                                  readUint32(body, 45),     // octets 60-63
		ProjectionCentreFlag:                body[49],                 // octet 64
		ScanningMode:                        body[50],                 // octet 65
	}, nil
}

func writeTemplate320(t Template320) []byte {
	buf := make([]byte, template320Size)
	buf[0] = t.ShapeOfEarth
	buf[1] = t.ScaleFactorOfRadiusOfSphericalEarth
	binary.BigEndian.PutUint32(buf[2:6], t.ScaledValueOfRadiusOfSphericalEarth)
	buf[6] = t.ScaleFactorOfEarthMajorAxis
	binary.BigEndian.PutUint32(buf[7:11], t.ScaledValueOfEarthMajorAxis)
	buf[11] = t.ScaleFactorOfEarthMinorAxis
	binary.BigEndian.PutUint32(buf[12:16], t.ScaledValueOfEarthMinorAxis)
	binary.BigEndian.PutUint32(buf[16:20], t.Nx)
	binary.BigEndian.PutUint32(buf[20:24], t.Ny)
	binary.BigEndian.PutUint32(buf[24:28], writeSignMag32(t.LatitudeOfFirstGridPoint))
	binary.BigEndian.PutUint32(buf[28:32], writeSignMag32(t.LongitudeOfFirstGridPoint))
	buf[32] = t.ResolutionAndComponentFlags
	binary.BigEndian.PutUint32(buf[33:37], writeSignMag32(t.LaD))
	binary.BigEndian.PutUint32(buf[37:41], writeSignMag32(t.OrientationOfTheGrid))
	binary.BigEndian.PutUint32(buf[41:45], t.Dx)
	binary.BigEndian.PutUint32(buf[45:49], t.Dy)
	buf[49] = t.ProjectionCentreFlag
	buf[50] = t.ScanningMode
	return buf
}

// Template 3.30 - Lambert Conformal Conic
// Template body is octets 15-81 = 67 bytes.
const template330Size = 67

func readTemplate330(body []byte) (Template330, error) {
	if len(body) < template330Size {
		return Template330{}, fmt.Errorf("grib2: template 3.30 too short: %d bytes, need %d", len(body), template330Size)
	}

	return Template330{
		ShapeOfEarth:                        body[0],                  // octet 15
		ScaleFactorOfRadiusOfSphericalEarth: body[1],                  // octet 16
		ScaledValueOfRadiusOfSphericalEarth: readUint32(body, 2),      // octets 17-20
		ScaleFactorOfEarthMajorAxis:         body[6],                  // octet 21
		ScaledValueOfEarthMajorAxis:         readUint32(body, 7),      // octets 22-25
		ScaleFactorOfEarthMinorAxis:         body[11],                 // octet 26
		ScaledValueOfEarthMinorAxis:         readUint32(body, 12),     // octets 27-30
		Nx:                                  readUint32(body, 16),     // octets 31-34
		Ny:                                  readUint32(body, 20),     // octets 35-38
		LatitudeOfFirstGridPoint:            readSignMag32(body, 24),  // octets 39-42
		LongitudeOfFirstGridPoint:           readSignMag32(body, 28),  // octets 43-46
		ResolutionAndComponentFlags:         body[32],                 // octet 47
		LaD:                                 readSignMag32(body, 33),  // octets 48-51
		LoV:                                 readSignMag32(body, 37),  // octets 52-55
		Dx:                                  readUint32(body, 41),     // octets 56-59
		Dy:                                  readUint32(body, 45),     // octets 60-63
		ProjectionCentreFlag:                body[49],                 // octet 64
		ScanningMode:                        body[50],                 // octet 65
		Latin1:                              readSignMag32(body, 51),  // octets 66-69
		Latin2:                              readSignMag32(body, 55),  // octets 70-73
		LatitudeOfSouthernPole:              readSignMag32(body, 59),  // octets 74-77
		LongitudeOfSouthernPole:             readSignMag32(body, 63),  // octets 78-81
	}, nil
}

func writeTemplate330(t Template330) []byte {
	buf := make([]byte, template330Size)
	buf[0] = t.ShapeOfEarth
	buf[1] = t.ScaleFactorOfRadiusOfSphericalEarth
	binary.BigEndian.PutUint32(buf[2:6], t.ScaledValueOfRadiusOfSphericalEarth)
	buf[6] = t.ScaleFactorOfEarthMajorAxis
	binary.BigEndian.PutUint32(buf[7:11], t.ScaledValueOfEarthMajorAxis)
	buf[11] = t.ScaleFactorOfEarthMinorAxis
	binary.BigEndian.PutUint32(buf[12:16], t.ScaledValueOfEarthMinorAxis)
	binary.BigEndian.PutUint32(buf[16:20], t.Nx)
	binary.BigEndian.PutUint32(buf[20:24], t.Ny)
	binary.BigEndian.PutUint32(buf[24:28], writeSignMag32(t.LatitudeOfFirstGridPoint))
	binary.BigEndian.PutUint32(buf[28:32], writeSignMag32(t.LongitudeOfFirstGridPoint))
	buf[32] = t.ResolutionAndComponentFlags
	binary.BigEndian.PutUint32(buf[33:37], writeSignMag32(t.LaD))
	binary.BigEndian.PutUint32(buf[37:41], writeSignMag32(t.LoV))
	binary.BigEndian.PutUint32(buf[41:45], t.Dx)
	binary.BigEndian.PutUint32(buf[45:49], t.Dy)
	buf[49] = t.ProjectionCentreFlag
	buf[50] = t.ScanningMode
	binary.BigEndian.PutUint32(buf[51:55], writeSignMag32(t.Latin1))
	binary.BigEndian.PutUint32(buf[55:59], writeSignMag32(t.Latin2))
	binary.BigEndian.PutUint32(buf[59:63], writeSignMag32(t.LatitudeOfSouthernPole))
	binary.BigEndian.PutUint32(buf[63:67], writeSignMag32(t.LongitudeOfSouthernPole))
	return buf
}

// Template 3.40 - Gaussian Latitude/Longitude
// Template body is octets 15-72 = 58 bytes (same size as 3.0).
// Any optional PL list follows after octet 72.
const template340Size = 58

func readTemplate340(body []byte, octetsPerPL uint8) (Template340, []byte, error) {
	if len(body) < template340Size {
		return Template340{}, nil, fmt.Errorf("grib2: template 3.40 too short: %d bytes, need %d", len(body), template340Size)
	}

	tmpl := Template340{
		ShapeOfEarth:                        body[0],                  // octet 15
		ScaleFactorOfRadiusOfSphericalEarth: body[1],                  // octet 16
		ScaledValueOfRadiusOfSphericalEarth: readUint32(body, 2),      // octets 17-20
		ScaleFactorOfEarthMajorAxis:         body[6],                  // octet 21
		ScaledValueOfEarthMajorAxis:         readUint32(body, 7),      // octets 22-25
		ScaleFactorOfEarthMinorAxis:         body[11],                 // octet 26
		ScaledValueOfEarthMinorAxis:         readUint32(body, 12),     // octets 27-30
		Ni:                                  readUint32(body, 16),     // octets 31-34
		Nj:                                  readUint32(body, 20),     // octets 35-38
		BasicAngle:                          readUint32(body, 24),     // octets 39-42
		SubdivisionsOfBasicAngle:            readUint32(body, 28),     // octets 43-46
		LatitudeOfFirstGridPoint:            readSignMag32(body, 32),  // octets 47-50
		LongitudeOfFirstGridPoint:           readSignMag32(body, 36),  // octets 51-54
		ResolutionAndComponentFlags:         body[40],                 // octet 55
		LatitudeOfLastGridPoint:             readSignMag32(body, 41),  // octets 56-59
		LongitudeOfLastGridPoint:            readSignMag32(body, 45),  // octets 60-63
		IDirectionIncrement:                 readUint32(body, 49),     // octets 64-67
		N:                                   readUint32(body, 53),     // octets 68-71
		ScanningMode:                        body[57],                 // octet 72
	}

	// Optional PL array follows the template (octets 73+)
	var optPL []byte
	if len(body) > template340Size {
		optPL = append([]byte(nil), body[template340Size:]...)
	}

	return tmpl, optPL, nil
}

func writeTemplate340(t Template340) []byte {
	buf := make([]byte, template340Size)
	buf[0] = t.ShapeOfEarth
	buf[1] = t.ScaleFactorOfRadiusOfSphericalEarth
	binary.BigEndian.PutUint32(buf[2:6], t.ScaledValueOfRadiusOfSphericalEarth)
	buf[6] = t.ScaleFactorOfEarthMajorAxis
	binary.BigEndian.PutUint32(buf[7:11], t.ScaledValueOfEarthMajorAxis)
	buf[11] = t.ScaleFactorOfEarthMinorAxis
	binary.BigEndian.PutUint32(buf[12:16], t.ScaledValueOfEarthMinorAxis)
	binary.BigEndian.PutUint32(buf[16:20], t.Ni)
	binary.BigEndian.PutUint32(buf[20:24], t.Nj)
	binary.BigEndian.PutUint32(buf[24:28], t.BasicAngle)
	binary.BigEndian.PutUint32(buf[28:32], t.SubdivisionsOfBasicAngle)
	binary.BigEndian.PutUint32(buf[32:36], writeSignMag32(t.LatitudeOfFirstGridPoint))
	binary.BigEndian.PutUint32(buf[36:40], writeSignMag32(t.LongitudeOfFirstGridPoint))
	buf[40] = t.ResolutionAndComponentFlags
	binary.BigEndian.PutUint32(buf[41:45], writeSignMag32(t.LatitudeOfLastGridPoint))
	binary.BigEndian.PutUint32(buf[45:49], writeSignMag32(t.LongitudeOfLastGridPoint))
	binary.BigEndian.PutUint32(buf[49:53], t.IDirectionIncrement)
	binary.BigEndian.PutUint32(buf[53:57], t.N)
	buf[57] = t.ScanningMode
	return buf
}

// Template 3.90 - Space View Perspective or Orthographic
// Template body is octets 15-80 = 66 bytes.
const template390Size = 66

func readTemplate390(body []byte) (Template390, error) {
	if len(body) < template390Size {
		return Template390{}, fmt.Errorf("grib2: template 3.90 too short: %d bytes, need %d", len(body), template390Size)
	}

	return Template390{
		ShapeOfEarth:                        body[0],                  // octet 15
		ScaleFactorOfRadiusOfSphericalEarth: body[1],                  // octet 16
		ScaledValueOfRadiusOfSphericalEarth: readUint32(body, 2),      // octets 17-20
		ScaleFactorOfEarthMajorAxis:         body[6],                  // octet 21
		ScaledValueOfEarthMajorAxis:         readUint32(body, 7),      // octets 22-25
		ScaleFactorOfEarthMinorAxis:         body[11],                 // octet 26
		ScaledValueOfEarthMinorAxis:         readUint32(body, 12),     // octets 27-30
		Nx:                                  readUint32(body, 16),     // octets 31-34
		Ny:                                  readUint32(body, 20),     // octets 35-38
		LatitudeOfSubSatellitePoint:         readSignMag32(body, 24),  // octets 39-42
		LongitudeOfSubSatellitePoint:        readSignMag32(body, 28),  // octets 43-46
		ResolutionAndComponentFlags:         body[32],                 // octet 47
		Dx:                                  readUint32(body, 33),     // octets 48-51
		Dy:                                  readUint32(body, 37),     // octets 52-55
		Xp:                                  readUint32(body, 41),     // octets 56-59
		Yp:                                  readUint32(body, 45),     // octets 60-63
		ScanningMode:                        body[49],                 // octet 64
		OrientationOfTheGrid:                readSignMag32(body, 50),  // octets 65-68
		Nr:                                  readUint32(body, 54),     // octets 69-72
		Xo:                                  readUint32(body, 58),     // octets 73-76
		Yo:                                  readUint32(body, 62),     // octets 77-80
	}, nil
}

func writeTemplate390(t Template390) []byte {
	buf := make([]byte, template390Size)
	buf[0] = t.ShapeOfEarth
	buf[1] = t.ScaleFactorOfRadiusOfSphericalEarth
	binary.BigEndian.PutUint32(buf[2:6], t.ScaledValueOfRadiusOfSphericalEarth)
	buf[6] = t.ScaleFactorOfEarthMajorAxis
	binary.BigEndian.PutUint32(buf[7:11], t.ScaledValueOfEarthMajorAxis)
	buf[11] = t.ScaleFactorOfEarthMinorAxis
	binary.BigEndian.PutUint32(buf[12:16], t.ScaledValueOfEarthMinorAxis)
	binary.BigEndian.PutUint32(buf[16:20], t.Nx)
	binary.BigEndian.PutUint32(buf[20:24], t.Ny)
	binary.BigEndian.PutUint32(buf[24:28], writeSignMag32(t.LatitudeOfSubSatellitePoint))
	binary.BigEndian.PutUint32(buf[28:32], writeSignMag32(t.LongitudeOfSubSatellitePoint))
	buf[32] = t.ResolutionAndComponentFlags
	binary.BigEndian.PutUint32(buf[33:37], t.Dx)
	binary.BigEndian.PutUint32(buf[37:41], t.Dy)
	binary.BigEndian.PutUint32(buf[41:45], t.Xp)
	binary.BigEndian.PutUint32(buf[45:49], t.Yp)
	buf[49] = t.ScanningMode
	binary.BigEndian.PutUint32(buf[50:54], writeSignMag32(t.OrientationOfTheGrid))
	binary.BigEndian.PutUint32(buf[54:58], t.Nr)
	binary.BigEndian.PutUint32(buf[58:62], t.Xo)
	binary.BigEndian.PutUint32(buf[62:66], t.Yo)
	return buf
}
