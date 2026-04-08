package grib2

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Section4 is the Product Definition Section.
//
// Common header:
//
//	1-4    Section length
//	5      Number of section (4)
//	6-7    Number of coordinate values after template (NV)
//	8-9    Product definition template number (Table 4.0)
//	10-N   Product definition template
type Section4 struct {
	Length           uint32
	NV               uint16 // number of coordinate values
	TemplateNumber   uint16 // Table 4.0
	Template         ProductDefinitionTemplate
	CoordinateValues []byte // NV coordinate values (if any)
}

// ProductDefinitionTemplate is the interface for product definition templates.
type ProductDefinitionTemplate interface {
	TemplateNumber() uint16
}

// Template40 is Product Definition Template 4.0:
// Analysis or forecast at a horizontal level or in a horizontal
// layer at a point in time.
//
// Octets 10-34 (relative to section start).
type Template40 struct {
	ParameterCategory         uint8  // Table 4.1
	ParameterNumber           uint8  // Table 4.2
	TypeOfGeneratingProcess   uint8  // Table 4.3
	BackgroundProcess         uint8
	GeneratingProcessID       uint8
	HoursAfterCutoff          uint16
	MinutesAfterCutoff        uint8
	IndicatorOfUnitOfTimeRange uint8 // Table 4.4
	ForecastTime              int32
	TypeOfFirstFixedSurface   uint8  // Table 4.5
	ScaleFactorOfFirstSurface int8
	ScaledValueOfFirstSurface uint32
	TypeOfSecondFixedSurface  uint8  // Table 4.5
	ScaleFactorOfSecondSurface int8
	ScaledValueOfSecondSurface uint32
}

func (Template40) TemplateNumber() uint16 { return 0 }

// Template41 is Product Definition Template 4.1:
// Individual ensemble forecast, control and perturbed, at a horizontal
// level or in a horizontal layer at a point in time.
// It extends Template 4.0 with 3 ensemble fields.
type Template41 struct {
	Template40
	TypeOfEnsembleForecast      uint8 // Table 4.6
	PerturbationNumber          uint8
	NumberOfForecastsInEnsemble uint8
}

func (Template41) TemplateNumber() uint16 { return 1 }

// Template42 is Product Definition Template 4.2:
// Derived forecasts based on all ensemble members at a horizontal
// level or in a horizontal layer at a point in time.
// It extends Template 4.0 with 2 derived-forecast fields.
type Template42 struct {
	Template40
	DerivedForecast             uint8 // Table 4.7 — 0=unweighted mean, 1=weighted mean, 2=std dev, etc.
	NumberOfForecastsInEnsemble uint8
}

func (Template42) TemplateNumber() uint16 { return 2 }

// TimeRangeSpec describes one statistical processing time range
// within Template 4.8.
type TimeRangeSpec struct {
	TypeOfStatisticalProcessing      uint8  // Table 4.10
	TypeOfTimeIncrement              uint8  // Table 4.11
	IndicatorOfUnitForTimeRange      uint8  // Table 4.4
	LengthOfTimeRange                uint32
	IndicatorOfUnitForTimeIncrement  uint8  // Table 4.4
	TimeIncrement                    uint32
}

const timeRangeSpecSize = 12 // 1+1+1+4+1+4 = 12 bytes per time range spec

// Template48 is Product Definition Template 4.8:
// Average, accumulation, extreme values or other statistically
// processed values at a horizontal level or in a horizontal layer
// in a continuous or non-continuous time interval.
//
// Extends Template 4.0 with end-of-interval time and time range specifications.
//
// Octets 10-34 are Template 4.0, then:
//
//	35-36  Year of end of overall time interval
//	37     Month of end of overall time interval
//	38     Day of end of overall time interval
//	39     Hour of end of overall time interval
//	40     Minute of end of overall time interval
//	41     Second of end of overall time interval
//	42     Number of time range specifications (n)
//	43-46  Total number of data values missing in statistical process
//	47-58  First time range spec (12 bytes each)
//	...    Additional time range specs
type Template48 struct {
	Template40
	YearOfEndOfInterval              uint16
	MonthOfEndOfInterval             uint8
	DayOfEndOfInterval               uint8
	HourOfEndOfInterval              uint8
	MinuteOfEndOfInterval            uint8
	SecondOfEndOfInterval            uint8
	NumberOfTimeRangeSpecs           uint8
	NumberOfMissingInStatisticalProcess uint32
	TimeRangeSpecs                   []TimeRangeSpec
}

func (Template48) TemplateNumber() uint16 { return 8 }

// Template411 is Product Definition Template 4.11:
// Individual ensemble forecast, control and perturbed, at a horizontal
// level or in a horizontal layer in a continuous or non-continuous time interval.
// Combines Template 4.1 (ensemble member info) + Template 4.8 (time interval).
//
// Octets 10-34 are Template 4.0, then:
//
//	35     Type of ensemble forecast (Table 4.6)
//	36     Perturbation number
//	37     Number of forecasts in ensemble
//	38-39  Year of end of overall time interval
//	40     Month of end of overall time interval
//	41     Day of end of overall time interval
//	42     Hour of end of overall time interval
//	43     Minute of end of overall time interval
//	44     Second of end of overall time interval
//	45     Number of time range specifications (n)
//	46-49  Total number of data values missing in statistical process
//	50-61  First time range spec (12 bytes each)
//	...    Additional time range specs
type Template411 struct {
	Template40
	TypeOfEnsembleForecast              uint8 // Table 4.6
	PerturbationNumber                  uint8
	NumberOfForecastsInEnsemble         uint8
	YearOfEndOfInterval                 uint16
	MonthOfEndOfInterval                uint8
	DayOfEndOfInterval                  uint8
	HourOfEndOfInterval                 uint8
	MinuteOfEndOfInterval               uint8
	SecondOfEndOfInterval               uint8
	NumberOfTimeRangeSpecs              uint8
	NumberOfMissingInStatisticalProcess uint32
	TimeRangeSpecs                      []TimeRangeSpec
}

func (Template411) TemplateNumber() uint16 { return 11 }

// template411FixedSize is the fixed part beyond template40:
// 3 (ensemble) + 12 (time interval header) = 15 bytes
const template411FixedSize = 15

// Template412 is Product Definition Template 4.12:
// Derived forecasts based on all ensemble members at a horizontal
// level or in a horizontal layer in a continuous or non-continuous time interval.
// Combines Template 4.2 (derived ensemble) + Template 4.8 (time interval).
//
// Octets 10-34 are Template 4.0, then:
//
//	35     Derived forecast type (Table 4.7)
//	36     Number of forecasts in ensemble
//	37-38  Year of end of overall time interval
//	39     Month of end of overall time interval
//	40     Day of end of overall time interval
//	41     Hour of end of overall time interval
//	42     Minute of end of overall time interval
//	43     Second of end of overall time interval
//	44     Number of time range specifications (n)
//	45-48  Total number of data values missing in statistical process
//	49-60  First time range spec (12 bytes each)
//	...    Additional time range specs
type Template412 struct {
	Template40
	DerivedForecast                     uint8 // Table 4.7
	NumberOfForecastsInEnsemble         uint8
	YearOfEndOfInterval                 uint16
	MonthOfEndOfInterval                uint8
	DayOfEndOfInterval                  uint8
	HourOfEndOfInterval                 uint8
	MinuteOfEndOfInterval               uint8
	SecondOfEndOfInterval               uint8
	NumberOfTimeRangeSpecs              uint8
	NumberOfMissingInStatisticalProcess uint32
	TimeRangeSpecs                      []TimeRangeSpec
}

func (Template412) TemplateNumber() uint16 { return 12 }

// template412FixedSize is the fixed part beyond template40:
// 2 (derived) + 12 (time interval header) = 14 bytes
const template412FixedSize = 14

// template48FixedSize is the fixed part beyond template40: 2+1+1+1+1+1+1+4 = 12 bytes
const template48FixedSize = 12

// RawProductTemplate stores an unparsed product definition template.
type RawProductTemplate struct {
	Number uint16
	Data   []byte
}

func (t *RawProductTemplate) TemplateNumber() uint16 { return t.Number }

const (
	template40Size = 25 // octets 10-34 relative to section = 25 bytes of template body
	template41Size = 28 // 25 + 3 ensemble bytes
	template42Size = 27 // 25 + 2 derived-forecast bytes
	// template48MinSize is the minimum size: template40 (25) + fixed interval fields (12) = 37 bytes (with 0 time range specs)
	template48MinSize    = template40Size + template48FixedSize
	template411MinSize   = template40Size + template411FixedSize
	template412MinSize   = template40Size + template412FixedSize
)

// ReadSection4 reads the Product Definition Section.
func ReadSection4(r io.Reader) (Section4, error) {
	length, secNum, body, err := readSectionHeader(r)
	if err != nil {
		return Section4{}, fmt.Errorf("grib2: section 4: %w", err)
	}
	if secNum != 4 {
		return Section4{}, fmt.Errorf("grib2: expected section 4, got section %d", secNum)
	}

	if len(body) < 4 {
		return Section4{}, fmt.Errorf("grib2: section 4 body too short: %d bytes", len(body))
	}

	s := Section4{
		Length:         length,
		NV:             readUint16(body, 0), // octets 6-7
		TemplateNumber: readUint16(body, 2), // octets 8-9
	}

	templateBody := body[4:] // starts at octet 10

	switch s.TemplateNumber {
	case 0:
		tmpl, err := readTemplate40(templateBody)
		if err != nil {
			return Section4{}, err
		}
		s.Template = tmpl
	case 1:
		tmpl, err := readTemplate41(templateBody)
		if err != nil {
			return Section4{}, err
		}
		s.Template = tmpl
	case 2:
		tmpl, err := readTemplate42(templateBody)
		if err != nil {
			return Section4{}, err
		}
		s.Template = tmpl
	case 8:
		tmpl, err := readTemplate48(templateBody)
		if err != nil {
			return Section4{}, err
		}
		s.Template = tmpl
	case 11:
		tmpl, err := readTemplate411(templateBody)
		if err != nil {
			return Section4{}, err
		}
		s.Template = tmpl
	case 12:
		tmpl, err := readTemplate412(templateBody)
		if err != nil {
			return Section4{}, err
		}
		s.Template = tmpl
	default:
		s.Template = &RawProductTemplate{
			Number: s.TemplateNumber,
			Data:   append([]byte(nil), templateBody...),
		}
	}

	return s, nil
}

func readTemplate40(body []byte) (Template40, error) {
	if len(body) < template40Size {
		return Template40{}, fmt.Errorf("grib2: template 4.0 too short: %d bytes, need %d", len(body), template40Size)
	}
	return Template40{
		ParameterCategory:          body[0],              // octet 10
		ParameterNumber:            body[1],              // octet 11
		TypeOfGeneratingProcess:    body[2],              // octet 12
		BackgroundProcess:          body[3],              // octet 13
		GeneratingProcessID:        body[4],              // octet 14
		HoursAfterCutoff:           readUint16(body, 5),  // octets 15-16
		MinutesAfterCutoff:         body[7],              // octet 17
		IndicatorOfUnitOfTimeRange: body[8],              // octet 18
		ForecastTime:               readInt32(body, 9),   // octets 19-22
		TypeOfFirstFixedSurface:    body[13],              // octet 23
		ScaleFactorOfFirstSurface:  int8(body[14]),       // octet 24
		ScaledValueOfFirstSurface:  readUint32(body, 15), // octets 25-28
		TypeOfSecondFixedSurface:   body[19],             // octet 29
		ScaleFactorOfSecondSurface: int8(body[20]),       // octet 30
		ScaledValueOfSecondSurface: readUint32(body, 21), // octets 31-34
	}, nil
}

func readTemplate41(body []byte) (Template41, error) {
	if len(body) < template41Size {
		return Template41{}, fmt.Errorf("grib2: template 4.1 too short: %d bytes, need %d", len(body), template41Size)
	}
	base, err := readTemplate40(body)
	if err != nil {
		return Template41{}, err
	}
	return Template41{
		Template40:                  base,
		TypeOfEnsembleForecast:      body[25], // octet 35
		PerturbationNumber:          body[26], // octet 36
		NumberOfForecastsInEnsemble: body[27], // octet 37
	}, nil
}

func readTemplate42(body []byte) (Template42, error) {
	if len(body) < template42Size {
		return Template42{}, fmt.Errorf("grib2: template 4.2 too short: %d bytes, need %d", len(body), template42Size)
	}
	base, err := readTemplate40(body)
	if err != nil {
		return Template42{}, err
	}
	return Template42{
		Template40:                  base,
		DerivedForecast:             body[25], // octet 35
		NumberOfForecastsInEnsemble: body[26], // octet 36
	}, nil
}

func readTemplate411(body []byte) (Template411, error) {
	if len(body) < template411MinSize {
		return Template411{}, fmt.Errorf("grib2: template 4.11 too short: %d bytes, need at least %d", len(body), template411MinSize)
	}
	base, err := readTemplate40(body)
	if err != nil {
		return Template411{}, err
	}
	off := template40Size // 25
	t := Template411{
		Template40:                          base,
		TypeOfEnsembleForecast:              body[off],               // octet 35
		PerturbationNumber:                  body[off+1],             // octet 36
		NumberOfForecastsInEnsemble:         body[off+2],             // octet 37
		YearOfEndOfInterval:                 readUint16(body, off+3), // octets 38-39
		MonthOfEndOfInterval:                body[off+5],             // octet 40
		DayOfEndOfInterval:                  body[off+6],             // octet 41
		HourOfEndOfInterval:                 body[off+7],             // octet 42
		MinuteOfEndOfInterval:               body[off+8],             // octet 43
		SecondOfEndOfInterval:               body[off+9],             // octet 44
		NumberOfTimeRangeSpecs:              body[off+10],            // octet 45
		NumberOfMissingInStatisticalProcess: readUint32(body, off+11), // octets 46-49
	}
	n := int(t.NumberOfTimeRangeSpecs)
	needed := template411MinSize + n*timeRangeSpecSize
	if len(body) < needed {
		return Template411{}, fmt.Errorf("grib2: template 4.11 too short for %d time ranges: %d bytes, need %d", n, len(body), needed)
	}
	t.TimeRangeSpecs = make([]TimeRangeSpec, n)
	trOff := template411MinSize
	for i := 0; i < n; i++ {
		t.TimeRangeSpecs[i] = TimeRangeSpec{
			TypeOfStatisticalProcessing:     body[trOff],
			TypeOfTimeIncrement:             body[trOff+1],
			IndicatorOfUnitForTimeRange:     body[trOff+2],
			LengthOfTimeRange:               readUint32(body, trOff+3),
			IndicatorOfUnitForTimeIncrement: body[trOff+7],
			TimeIncrement:                   readUint32(body, trOff+8),
		}
		trOff += timeRangeSpecSize
	}
	return t, nil
}

func readTemplate412(body []byte) (Template412, error) {
	if len(body) < template412MinSize {
		return Template412{}, fmt.Errorf("grib2: template 4.12 too short: %d bytes, need at least %d", len(body), template412MinSize)
	}
	base, err := readTemplate40(body)
	if err != nil {
		return Template412{}, err
	}
	off := template40Size // 25
	t := Template412{
		Template40:                          base,
		DerivedForecast:                     body[off],               // octet 35
		NumberOfForecastsInEnsemble:         body[off+1],             // octet 36
		YearOfEndOfInterval:                 readUint16(body, off+2), // octets 37-38
		MonthOfEndOfInterval:                body[off+4],             // octet 39
		DayOfEndOfInterval:                  body[off+5],             // octet 40
		HourOfEndOfInterval:                 body[off+6],             // octet 41
		MinuteOfEndOfInterval:               body[off+7],             // octet 42
		SecondOfEndOfInterval:               body[off+8],             // octet 43
		NumberOfTimeRangeSpecs:              body[off+9],             // octet 44
		NumberOfMissingInStatisticalProcess: readUint32(body, off+10), // octets 45-48
	}
	n := int(t.NumberOfTimeRangeSpecs)
	needed := template412MinSize + n*timeRangeSpecSize
	if len(body) < needed {
		return Template412{}, fmt.Errorf("grib2: template 4.12 too short for %d time ranges: %d bytes, need %d", n, len(body), needed)
	}
	t.TimeRangeSpecs = make([]TimeRangeSpec, n)
	trOff := template412MinSize
	for i := 0; i < n; i++ {
		t.TimeRangeSpecs[i] = TimeRangeSpec{
			TypeOfStatisticalProcessing:     body[trOff],
			TypeOfTimeIncrement:             body[trOff+1],
			IndicatorOfUnitForTimeRange:     body[trOff+2],
			LengthOfTimeRange:               readUint32(body, trOff+3),
			IndicatorOfUnitForTimeIncrement: body[trOff+7],
			TimeIncrement:                   readUint32(body, trOff+8),
		}
		trOff += timeRangeSpecSize
	}
	return t, nil
}

func readTemplate48(body []byte) (Template48, error) {
	if len(body) < template48MinSize {
		return Template48{}, fmt.Errorf("grib2: template 4.8 too short: %d bytes, need at least %d", len(body), template48MinSize)
	}
	base, err := readTemplate40(body)
	if err != nil {
		return Template48{}, err
	}
	off := template40Size // offset within template body, after template 4.0 fields
	t := Template48{
		Template40:                          base,
		YearOfEndOfInterval:                 readUint16(body, off),   // octets 35-36
		MonthOfEndOfInterval:                body[off+2],             // octet 37
		DayOfEndOfInterval:                  body[off+3],             // octet 38
		HourOfEndOfInterval:                 body[off+4],             // octet 39
		MinuteOfEndOfInterval:               body[off+5],             // octet 40
		SecondOfEndOfInterval:               body[off+6],             // octet 41
		NumberOfTimeRangeSpecs:              body[off+7],             // octet 42
		NumberOfMissingInStatisticalProcess: readUint32(body, off+8), // octets 43-46
	}
	n := int(t.NumberOfTimeRangeSpecs)
	needed := template48MinSize + n*timeRangeSpecSize
	if len(body) < needed {
		return Template48{}, fmt.Errorf("grib2: template 4.8 too short for %d time ranges: %d bytes, need %d", n, len(body), needed)
	}
	t.TimeRangeSpecs = make([]TimeRangeSpec, n)
	trOff := template48MinSize // start of first time range spec
	for i := 0; i < n; i++ {
		t.TimeRangeSpecs[i] = TimeRangeSpec{
			TypeOfStatisticalProcessing:     body[trOff],               // octet 47 + i*12
			TypeOfTimeIncrement:             body[trOff+1],             // octet 48 + i*12
			IndicatorOfUnitForTimeRange:     body[trOff+2],             // octet 49 + i*12
			LengthOfTimeRange:               readUint32(body, trOff+3), // octets 50-53 + i*12
			IndicatorOfUnitForTimeIncrement: body[trOff+7],             // octet 54 + i*12
			TimeIncrement:                   readUint32(body, trOff+8), // octets 55-58 + i*12
		}
		trOff += timeRangeSpecSize
	}
	return t, nil
}

func writeTemplate48(t Template48) []byte {
	n := len(t.TimeRangeSpecs)
	size := template48MinSize + n*timeRangeSpecSize
	buf := make([]byte, size)
	copy(buf[0:template40Size], writeTemplate40(t.Template40))
	off := template40Size
	binary.BigEndian.PutUint16(buf[off:off+2], t.YearOfEndOfInterval)
	buf[off+2] = t.MonthOfEndOfInterval
	buf[off+3] = t.DayOfEndOfInterval
	buf[off+4] = t.HourOfEndOfInterval
	buf[off+5] = t.MinuteOfEndOfInterval
	buf[off+6] = t.SecondOfEndOfInterval
	buf[off+7] = t.NumberOfTimeRangeSpecs
	binary.BigEndian.PutUint32(buf[off+8:off+12], t.NumberOfMissingInStatisticalProcess)
	trOff := template48MinSize
	for i := 0; i < n; i++ {
		tr := t.TimeRangeSpecs[i]
		buf[trOff] = tr.TypeOfStatisticalProcessing
		buf[trOff+1] = tr.TypeOfTimeIncrement
		buf[trOff+2] = tr.IndicatorOfUnitForTimeRange
		binary.BigEndian.PutUint32(buf[trOff+3:trOff+7], tr.LengthOfTimeRange)
		buf[trOff+7] = tr.IndicatorOfUnitForTimeIncrement
		binary.BigEndian.PutUint32(buf[trOff+8:trOff+12], tr.TimeIncrement)
		trOff += timeRangeSpecSize
	}
	return buf
}

// WriteSection4 writes the Product Definition Section.
func WriteSection4(w io.Writer, s Section4) error {
	var templateBytes []byte

	switch tmpl := s.Template.(type) {
	case Template40:
		templateBytes = writeTemplate40(tmpl)
	case Template41:
		templateBytes = writeTemplate41(tmpl)
	case Template42:
		templateBytes = writeTemplate42(tmpl)
	case Template48:
		templateBytes = writeTemplate48(tmpl)
	case Template411:
		templateBytes = writeTemplate411(tmpl)
	case Template412:
		templateBytes = writeTemplate412(tmpl)
	case *RawProductTemplate:
		templateBytes = tmpl.Data
	default:
		return fmt.Errorf("grib2: unsupported product template type %T", s.Template)
	}

	length := uint32(9 + len(templateBytes) + len(s.CoordinateValues))
	buf := make([]byte, length)

	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = 4
	binary.BigEndian.PutUint16(buf[5:7], s.NV)
	binary.BigEndian.PutUint16(buf[7:9], s.TemplateNumber)
	copy(buf[9:], templateBytes)
	if len(s.CoordinateValues) > 0 {
		copy(buf[9+len(templateBytes):], s.CoordinateValues)
	}

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("grib2: writing section 4: %w", err)
	}
	return nil
}

func writeTemplate40(t Template40) []byte {
	buf := make([]byte, template40Size)
	buf[0] = t.ParameterCategory
	buf[1] = t.ParameterNumber
	buf[2] = t.TypeOfGeneratingProcess
	buf[3] = t.BackgroundProcess
	buf[4] = t.GeneratingProcessID
	binary.BigEndian.PutUint16(buf[5:7], t.HoursAfterCutoff)
	buf[7] = t.MinutesAfterCutoff
	buf[8] = t.IndicatorOfUnitOfTimeRange
	binary.BigEndian.PutUint32(buf[9:13], uint32(t.ForecastTime))
	buf[13] = t.TypeOfFirstFixedSurface
	buf[14] = byte(t.ScaleFactorOfFirstSurface)
	binary.BigEndian.PutUint32(buf[15:19], t.ScaledValueOfFirstSurface)
	buf[19] = t.TypeOfSecondFixedSurface
	buf[20] = byte(t.ScaleFactorOfSecondSurface)
	binary.BigEndian.PutUint32(buf[21:25], t.ScaledValueOfSecondSurface)
	return buf
}

func writeTemplate41(t Template41) []byte {
	buf := writeTemplate40(t.Template40)
	buf = append(buf, t.TypeOfEnsembleForecast, t.PerturbationNumber, t.NumberOfForecastsInEnsemble)
	return buf
}

func writeTemplate42(t Template42) []byte {
	buf := writeTemplate40(t.Template40)
	buf = append(buf, t.DerivedForecast, t.NumberOfForecastsInEnsemble)
	return buf
}

func writeTemplate411(t Template411) []byte {
	n := len(t.TimeRangeSpecs)
	size := template411MinSize + n*timeRangeSpecSize
	buf := make([]byte, size)
	copy(buf[0:template40Size], writeTemplate40(t.Template40))
	off := template40Size
	buf[off] = t.TypeOfEnsembleForecast
	buf[off+1] = t.PerturbationNumber
	buf[off+2] = t.NumberOfForecastsInEnsemble
	binary.BigEndian.PutUint16(buf[off+3:off+5], t.YearOfEndOfInterval)
	buf[off+5] = t.MonthOfEndOfInterval
	buf[off+6] = t.DayOfEndOfInterval
	buf[off+7] = t.HourOfEndOfInterval
	buf[off+8] = t.MinuteOfEndOfInterval
	buf[off+9] = t.SecondOfEndOfInterval
	buf[off+10] = t.NumberOfTimeRangeSpecs
	binary.BigEndian.PutUint32(buf[off+11:off+15], t.NumberOfMissingInStatisticalProcess)
	trOff := template411MinSize
	for i := 0; i < n; i++ {
		tr := t.TimeRangeSpecs[i]
		buf[trOff] = tr.TypeOfStatisticalProcessing
		buf[trOff+1] = tr.TypeOfTimeIncrement
		buf[trOff+2] = tr.IndicatorOfUnitForTimeRange
		binary.BigEndian.PutUint32(buf[trOff+3:trOff+7], tr.LengthOfTimeRange)
		buf[trOff+7] = tr.IndicatorOfUnitForTimeIncrement
		binary.BigEndian.PutUint32(buf[trOff+8:trOff+12], tr.TimeIncrement)
		trOff += timeRangeSpecSize
	}
	return buf
}

func writeTemplate412(t Template412) []byte {
	n := len(t.TimeRangeSpecs)
	size := template412MinSize + n*timeRangeSpecSize
	buf := make([]byte, size)
	copy(buf[0:template40Size], writeTemplate40(t.Template40))
	off := template40Size
	buf[off] = t.DerivedForecast
	buf[off+1] = t.NumberOfForecastsInEnsemble
	binary.BigEndian.PutUint16(buf[off+2:off+4], t.YearOfEndOfInterval)
	buf[off+4] = t.MonthOfEndOfInterval
	buf[off+5] = t.DayOfEndOfInterval
	buf[off+6] = t.HourOfEndOfInterval
	buf[off+7] = t.MinuteOfEndOfInterval
	buf[off+8] = t.SecondOfEndOfInterval
	buf[off+9] = t.NumberOfTimeRangeSpecs
	binary.BigEndian.PutUint32(buf[off+10:off+14], t.NumberOfMissingInStatisticalProcess)
	trOff := template412MinSize
	for i := 0; i < n; i++ {
		tr := t.TimeRangeSpecs[i]
		buf[trOff] = tr.TypeOfStatisticalProcessing
		buf[trOff+1] = tr.TypeOfTimeIncrement
		buf[trOff+2] = tr.IndicatorOfUnitForTimeRange
		binary.BigEndian.PutUint32(buf[trOff+3:trOff+7], tr.LengthOfTimeRange)
		buf[trOff+7] = tr.IndicatorOfUnitForTimeIncrement
		binary.BigEndian.PutUint32(buf[trOff+8:trOff+12], tr.TimeIncrement)
		trOff += timeRangeSpecSize
	}
	return buf
}
