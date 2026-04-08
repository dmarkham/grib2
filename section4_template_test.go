package grib2

import (
	"bytes"
	"os"
	"testing"
)

// ---- Template 4.2 (Derived Ensemble Forecast) ----

func TestReadSection4_Template42(t *testing.T) {
	raw, err := os.ReadFile("testdata/template42.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section4.TemplateNumber != 2 {
		t.Fatalf("S4.TemplateNumber = %d, want 2", field.Section4.TemplateNumber)
	}

	tmpl, ok := field.Section4.Template.(Template42)
	if !ok {
		t.Fatalf("S4.Template type = %T, want Template42", field.Section4.Template)
	}

	// Cross-validated against: grib_dump -O testdata/template42.grib2
	if tmpl.ParameterCategory != 0 {
		t.Errorf("ParameterCategory = %d, want 0", tmpl.ParameterCategory)
	}
	if tmpl.ParameterNumber != 0 {
		t.Errorf("ParameterNumber = %d, want 0", tmpl.ParameterNumber)
	}
	if tmpl.GeneratingProcessID != 130 {
		t.Errorf("GeneratingProcessID = %d, want 130", tmpl.GeneratingProcessID)
	}
	if tmpl.TypeOfFirstFixedSurface != 103 {
		t.Errorf("TypeOfFirstFixedSurface = %d, want 103", tmpl.TypeOfFirstFixedSurface)
	}
	if tmpl.DerivedForecast != 0 {
		t.Errorf("DerivedForecast = %d, want 0 (Unweighted mean)", tmpl.DerivedForecast)
	}
	if tmpl.NumberOfForecastsInEnsemble != 30 {
		t.Errorf("NumberOfForecastsInEnsemble = %d, want 30", tmpl.NumberOfForecastsInEnsemble)
	}
}

func TestWriteSection4_Template42_RoundTrip(t *testing.T) {
	original := Section4{
		NV:             0,
		TemplateNumber: 2,
		Template: Template42{
			Template40: Template40{
				ParameterCategory:          0,
				ParameterNumber:            0,
				TypeOfGeneratingProcess:    0,
				BackgroundProcess:          0xFF,
				GeneratingProcessID:        130,
				HoursAfterCutoff:           0,
				MinutesAfterCutoff:         0,
				IndicatorOfUnitOfTimeRange: 1,
				ForecastTime:               12,
				TypeOfFirstFixedSurface:    103,
				ScaleFactorOfFirstSurface:  0,
				ScaledValueOfFirstSurface:  2,
				TypeOfSecondFixedSurface:   0xFF,
				ScaleFactorOfSecondSurface: -1,
				ScaledValueOfSecondSurface: 0xFFFFFFFF,
			},
			DerivedForecast:             2, // standard deviation
			NumberOfForecastsInEnsemble: 50,
		},
	}

	var buf bytes.Buffer
	if err := WriteSection4(&buf, original); err != nil {
		t.Fatalf("WriteSection4: %v", err)
	}

	decoded, err := ReadSection4(&buf)
	if err != nil {
		t.Fatalf("ReadSection4: %v", err)
	}

	if decoded.TemplateNumber != 2 {
		t.Fatalf("TemplateNumber = %d, want 2", decoded.TemplateNumber)
	}

	tmpl, ok := decoded.Template.(Template42)
	if !ok {
		t.Fatalf("Template type = %T, want Template42", decoded.Template)
	}
	orig := original.Template.(Template42)
	if tmpl != orig {
		t.Errorf("template mismatch:\n  got  %+v\n  want %+v", tmpl, orig)
	}
}

func TestWriteSection4_Template42_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/template42.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	s4 := msg.Fields[0].Section4
	var buf bytes.Buffer
	if err := WriteSection4(&buf, s4); err != nil {
		t.Fatalf("WriteSection4: %v", err)
	}

	got := buf.Bytes()
	// Find section 4 in the raw file by searching for it
	// Section 4 length is 36, so we search for that
	s4Offset := findSectionOffset(raw, 4)
	if s4Offset < 0 {
		t.Fatal("could not find section 4 in raw data")
	}
	s4Len := int(readUint32(raw, s4Offset))
	want := raw[s4Offset : s4Offset+s4Len]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// ---- Template 4.11 (Individual Ensemble + Time Interval) ----

func TestReadSection4_Template411(t *testing.T) {
	raw, err := os.ReadFile("testdata/template411.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section4.TemplateNumber != 11 {
		t.Fatalf("S4.TemplateNumber = %d, want 11", field.Section4.TemplateNumber)
	}

	tmpl, ok := field.Section4.Template.(Template411)
	if !ok {
		t.Fatalf("S4.Template type = %T, want Template411", field.Section4.Template)
	}

	// Cross-validated against: grib_dump -O testdata/template411.grib2
	if tmpl.ParameterCategory != 0 {
		t.Errorf("ParameterCategory = %d, want 0", tmpl.ParameterCategory)
	}
	if tmpl.ParameterNumber != 0 {
		t.Errorf("ParameterNumber = %d, want 0", tmpl.ParameterNumber)
	}
	if tmpl.GeneratingProcessID != 130 {
		t.Errorf("GeneratingProcessID = %d, want 130", tmpl.GeneratingProcessID)
	}
	if tmpl.TypeOfEnsembleForecast != 3 {
		t.Errorf("TypeOfEnsembleForecast = %d, want 3", tmpl.TypeOfEnsembleForecast)
	}
	if tmpl.PerturbationNumber != 5 {
		t.Errorf("PerturbationNumber = %d, want 5", tmpl.PerturbationNumber)
	}
	if tmpl.NumberOfForecastsInEnsemble != 50 {
		t.Errorf("NumberOfForecastsInEnsemble = %d, want 50", tmpl.NumberOfForecastsInEnsemble)
	}
	if tmpl.YearOfEndOfInterval != 2023 {
		t.Errorf("YearOfEndOfInterval = %d, want 2023", tmpl.YearOfEndOfInterval)
	}
	if tmpl.MonthOfEndOfInterval != 6 {
		t.Errorf("MonthOfEndOfInterval = %d, want 6", tmpl.MonthOfEndOfInterval)
	}
	if tmpl.DayOfEndOfInterval != 15 {
		t.Errorf("DayOfEndOfInterval = %d, want 15", tmpl.DayOfEndOfInterval)
	}
	if tmpl.HourOfEndOfInterval != 12 {
		t.Errorf("HourOfEndOfInterval = %d, want 12", tmpl.HourOfEndOfInterval)
	}
	if tmpl.MinuteOfEndOfInterval != 30 {
		t.Errorf("MinuteOfEndOfInterval = %d, want 30", tmpl.MinuteOfEndOfInterval)
	}
	if tmpl.SecondOfEndOfInterval != 0 {
		t.Errorf("SecondOfEndOfInterval = %d, want 0", tmpl.SecondOfEndOfInterval)
	}
	if tmpl.NumberOfTimeRangeSpecs != 1 {
		t.Fatalf("NumberOfTimeRangeSpecs = %d, want 1", tmpl.NumberOfTimeRangeSpecs)
	}
	if tmpl.NumberOfMissingInStatisticalProcess != 0 {
		t.Errorf("NumberOfMissingInStatisticalProcess = %d, want 0", tmpl.NumberOfMissingInStatisticalProcess)
	}
	if len(tmpl.TimeRangeSpecs) != 1 {
		t.Fatalf("len(TimeRangeSpecs) = %d, want 1", len(tmpl.TimeRangeSpecs))
	}
	tr := tmpl.TimeRangeSpecs[0]
	if tr.TypeOfStatisticalProcessing != 1 {
		t.Errorf("TypeOfStatisticalProcessing = %d, want 1 (Accumulation)", tr.TypeOfStatisticalProcessing)
	}
	if tr.TypeOfTimeIncrement != 2 {
		t.Errorf("TypeOfTimeIncrement = %d, want 2", tr.TypeOfTimeIncrement)
	}
	if tr.IndicatorOfUnitForTimeRange != 1 {
		t.Errorf("IndicatorOfUnitForTimeRange = %d, want 1", tr.IndicatorOfUnitForTimeRange)
	}
	if tr.LengthOfTimeRange != 6 {
		t.Errorf("LengthOfTimeRange = %d, want 6", tr.LengthOfTimeRange)
	}
	if tr.IndicatorOfUnitForTimeIncrement != 1 {
		t.Errorf("IndicatorOfUnitForTimeIncrement = %d, want 1", tr.IndicatorOfUnitForTimeIncrement)
	}
	if tr.TimeIncrement != 0 {
		t.Errorf("TimeIncrement = %d, want 0", tr.TimeIncrement)
	}
}

func TestWriteSection4_Template411_RoundTrip(t *testing.T) {
	original := Section4{
		NV:             0,
		TemplateNumber: 11,
		Template: Template411{
			Template40: Template40{
				ParameterCategory:          0,
				ParameterNumber:            0,
				TypeOfGeneratingProcess:    0,
				BackgroundProcess:          0xFF,
				GeneratingProcessID:        130,
				HoursAfterCutoff:           0,
				MinutesAfterCutoff:         0,
				IndicatorOfUnitOfTimeRange: 1,
				ForecastTime:               0,
				TypeOfFirstFixedSurface:    103,
				ScaleFactorOfFirstSurface:  -1,
				ScaledValueOfFirstSurface:  0xFFFFFFFF,
				TypeOfSecondFixedSurface:   0xFF,
				ScaleFactorOfSecondSurface: -1,
				ScaledValueOfSecondSurface: 0xFFFFFFFF,
			},
			TypeOfEnsembleForecast:              3,
			PerturbationNumber:                  5,
			NumberOfForecastsInEnsemble:         50,
			YearOfEndOfInterval:                 2023,
			MonthOfEndOfInterval:                6,
			DayOfEndOfInterval:                  15,
			HourOfEndOfInterval:                 12,
			MinuteOfEndOfInterval:               30,
			SecondOfEndOfInterval:               0,
			NumberOfTimeRangeSpecs:              1,
			NumberOfMissingInStatisticalProcess: 0,
			TimeRangeSpecs: []TimeRangeSpec{
				{
					TypeOfStatisticalProcessing:     1,
					TypeOfTimeIncrement:             2,
					IndicatorOfUnitForTimeRange:     1,
					LengthOfTimeRange:               6,
					IndicatorOfUnitForTimeIncrement: 1,
					TimeIncrement:                   0,
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteSection4(&buf, original); err != nil {
		t.Fatalf("WriteSection4: %v", err)
	}

	decoded, err := ReadSection4(&buf)
	if err != nil {
		t.Fatalf("ReadSection4: %v", err)
	}

	if decoded.TemplateNumber != 11 {
		t.Fatalf("TemplateNumber = %d, want 11", decoded.TemplateNumber)
	}

	tmpl, ok := decoded.Template.(Template411)
	if !ok {
		t.Fatalf("Template type = %T, want Template411", decoded.Template)
	}
	orig := original.Template.(Template411)

	// Compare non-slice fields
	if tmpl.Template40 != orig.Template40 {
		t.Errorf("Template40 mismatch:\n  got  %+v\n  want %+v", tmpl.Template40, orig.Template40)
	}
	if tmpl.TypeOfEnsembleForecast != orig.TypeOfEnsembleForecast {
		t.Errorf("TypeOfEnsembleForecast = %d, want %d", tmpl.TypeOfEnsembleForecast, orig.TypeOfEnsembleForecast)
	}
	if tmpl.PerturbationNumber != orig.PerturbationNumber {
		t.Errorf("PerturbationNumber = %d, want %d", tmpl.PerturbationNumber, orig.PerturbationNumber)
	}
	if tmpl.NumberOfForecastsInEnsemble != orig.NumberOfForecastsInEnsemble {
		t.Errorf("NumberOfForecastsInEnsemble = %d, want %d", tmpl.NumberOfForecastsInEnsemble, orig.NumberOfForecastsInEnsemble)
	}
	if tmpl.YearOfEndOfInterval != orig.YearOfEndOfInterval {
		t.Errorf("YearOfEndOfInterval = %d, want %d", tmpl.YearOfEndOfInterval, orig.YearOfEndOfInterval)
	}
	if tmpl.NumberOfTimeRangeSpecs != orig.NumberOfTimeRangeSpecs {
		t.Fatalf("NumberOfTimeRangeSpecs = %d, want %d", tmpl.NumberOfTimeRangeSpecs, orig.NumberOfTimeRangeSpecs)
	}
	if len(tmpl.TimeRangeSpecs) != len(orig.TimeRangeSpecs) {
		t.Fatalf("len(TimeRangeSpecs) = %d, want %d", len(tmpl.TimeRangeSpecs), len(orig.TimeRangeSpecs))
	}
	for i, tr := range tmpl.TimeRangeSpecs {
		if tr != orig.TimeRangeSpecs[i] {
			t.Errorf("TimeRangeSpecs[%d] mismatch:\n  got  %+v\n  want %+v", i, tr, orig.TimeRangeSpecs[i])
		}
	}
}

func TestWriteSection4_Template411_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/template411.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	s4 := msg.Fields[0].Section4
	var buf bytes.Buffer
	if err := WriteSection4(&buf, s4); err != nil {
		t.Fatalf("WriteSection4: %v", err)
	}

	got := buf.Bytes()
	s4Offset := findSectionOffset(raw, 4)
	if s4Offset < 0 {
		t.Fatal("could not find section 4 in raw data")
	}
	s4Len := int(readUint32(raw, s4Offset))
	want := raw[s4Offset : s4Offset+s4Len]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// ---- Template 4.12 (Derived Ensemble + Time Interval) ----

func TestReadSection4_Template412(t *testing.T) {
	raw, err := os.ReadFile("testdata/template412.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section4.TemplateNumber != 12 {
		t.Fatalf("S4.TemplateNumber = %d, want 12", field.Section4.TemplateNumber)
	}

	tmpl, ok := field.Section4.Template.(Template412)
	if !ok {
		t.Fatalf("S4.Template type = %T, want Template412", field.Section4.Template)
	}

	// Cross-validated against: grib_dump -O testdata/template412.grib2
	if tmpl.ParameterCategory != 0 {
		t.Errorf("ParameterCategory = %d, want 0", tmpl.ParameterCategory)
	}
	if tmpl.ParameterNumber != 0 {
		t.Errorf("ParameterNumber = %d, want 0", tmpl.ParameterNumber)
	}
	if tmpl.DerivedForecast != 2 {
		t.Errorf("DerivedForecast = %d, want 2 (Standard deviation)", tmpl.DerivedForecast)
	}
	if tmpl.NumberOfForecastsInEnsemble != 21 {
		t.Errorf("NumberOfForecastsInEnsemble = %d, want 21", tmpl.NumberOfForecastsInEnsemble)
	}
	if tmpl.YearOfEndOfInterval != 2024 {
		t.Errorf("YearOfEndOfInterval = %d, want 2024", tmpl.YearOfEndOfInterval)
	}
	if tmpl.MonthOfEndOfInterval != 1 {
		t.Errorf("MonthOfEndOfInterval = %d, want 1", tmpl.MonthOfEndOfInterval)
	}
	if tmpl.DayOfEndOfInterval != 20 {
		t.Errorf("DayOfEndOfInterval = %d, want 20", tmpl.DayOfEndOfInterval)
	}
	if tmpl.HourOfEndOfInterval != 6 {
		t.Errorf("HourOfEndOfInterval = %d, want 6", tmpl.HourOfEndOfInterval)
	}
	if tmpl.MinuteOfEndOfInterval != 0 {
		t.Errorf("MinuteOfEndOfInterval = %d, want 0", tmpl.MinuteOfEndOfInterval)
	}
	if tmpl.SecondOfEndOfInterval != 0 {
		t.Errorf("SecondOfEndOfInterval = %d, want 0", tmpl.SecondOfEndOfInterval)
	}
	if tmpl.NumberOfTimeRangeSpecs != 1 {
		t.Fatalf("NumberOfTimeRangeSpecs = %d, want 1", tmpl.NumberOfTimeRangeSpecs)
	}
	if tmpl.NumberOfMissingInStatisticalProcess != 0 {
		t.Errorf("NumberOfMissingInStatisticalProcess = %d, want 0", tmpl.NumberOfMissingInStatisticalProcess)
	}
	if len(tmpl.TimeRangeSpecs) != 1 {
		t.Fatalf("len(TimeRangeSpecs) = %d, want 1", len(tmpl.TimeRangeSpecs))
	}
	tr := tmpl.TimeRangeSpecs[0]
	if tr.TypeOfStatisticalProcessing != 0 {
		t.Errorf("TypeOfStatisticalProcessing = %d, want 0 (Average)", tr.TypeOfStatisticalProcessing)
	}
	if tr.TypeOfTimeIncrement != 2 {
		t.Errorf("TypeOfTimeIncrement = %d, want 2", tr.TypeOfTimeIncrement)
	}
	if tr.IndicatorOfUnitForTimeRange != 1 {
		t.Errorf("IndicatorOfUnitForTimeRange = %d, want 1", tr.IndicatorOfUnitForTimeRange)
	}
	if tr.LengthOfTimeRange != 24 {
		t.Errorf("LengthOfTimeRange = %d, want 24", tr.LengthOfTimeRange)
	}
	if tr.IndicatorOfUnitForTimeIncrement != 255 {
		t.Errorf("IndicatorOfUnitForTimeIncrement = %d, want 255", tr.IndicatorOfUnitForTimeIncrement)
	}
	if tr.TimeIncrement != 0 {
		t.Errorf("TimeIncrement = %d, want 0", tr.TimeIncrement)
	}
}

func TestWriteSection4_Template412_RoundTrip(t *testing.T) {
	original := Section4{
		NV:             0,
		TemplateNumber: 12,
		Template: Template412{
			Template40: Template40{
				ParameterCategory:          0,
				ParameterNumber:            0,
				TypeOfGeneratingProcess:    0,
				BackgroundProcess:          0xFF,
				GeneratingProcessID:        130,
				HoursAfterCutoff:           0,
				MinutesAfterCutoff:         0,
				IndicatorOfUnitOfTimeRange: 1,
				ForecastTime:               0,
				TypeOfFirstFixedSurface:    103,
				ScaleFactorOfFirstSurface:  -1,
				ScaledValueOfFirstSurface:  0xFFFFFFFF,
				TypeOfSecondFixedSurface:   0xFF,
				ScaleFactorOfSecondSurface: -1,
				ScaledValueOfSecondSurface: 0xFFFFFFFF,
			},
			DerivedForecast:                     2,
			NumberOfForecastsInEnsemble:         21,
			YearOfEndOfInterval:                 2024,
			MonthOfEndOfInterval:                1,
			DayOfEndOfInterval:                  20,
			HourOfEndOfInterval:                 6,
			MinuteOfEndOfInterval:               0,
			SecondOfEndOfInterval:               0,
			NumberOfTimeRangeSpecs:              1,
			NumberOfMissingInStatisticalProcess: 0,
			TimeRangeSpecs: []TimeRangeSpec{
				{
					TypeOfStatisticalProcessing:     0,
					TypeOfTimeIncrement:             2,
					IndicatorOfUnitForTimeRange:     1,
					LengthOfTimeRange:               24,
					IndicatorOfUnitForTimeIncrement: 255,
					TimeIncrement:                   0,
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteSection4(&buf, original); err != nil {
		t.Fatalf("WriteSection4: %v", err)
	}

	decoded, err := ReadSection4(&buf)
	if err != nil {
		t.Fatalf("ReadSection4: %v", err)
	}

	if decoded.TemplateNumber != 12 {
		t.Fatalf("TemplateNumber = %d, want 12", decoded.TemplateNumber)
	}

	tmpl, ok := decoded.Template.(Template412)
	if !ok {
		t.Fatalf("Template type = %T, want Template412", decoded.Template)
	}
	orig := original.Template.(Template412)

	if tmpl.Template40 != orig.Template40 {
		t.Errorf("Template40 mismatch:\n  got  %+v\n  want %+v", tmpl.Template40, orig.Template40)
	}
	if tmpl.DerivedForecast != orig.DerivedForecast {
		t.Errorf("DerivedForecast = %d, want %d", tmpl.DerivedForecast, orig.DerivedForecast)
	}
	if tmpl.NumberOfForecastsInEnsemble != orig.NumberOfForecastsInEnsemble {
		t.Errorf("NumberOfForecastsInEnsemble = %d, want %d", tmpl.NumberOfForecastsInEnsemble, orig.NumberOfForecastsInEnsemble)
	}
	if tmpl.YearOfEndOfInterval != orig.YearOfEndOfInterval {
		t.Errorf("YearOfEndOfInterval = %d, want %d", tmpl.YearOfEndOfInterval, orig.YearOfEndOfInterval)
	}
	if tmpl.NumberOfTimeRangeSpecs != orig.NumberOfTimeRangeSpecs {
		t.Fatalf("NumberOfTimeRangeSpecs = %d, want %d", tmpl.NumberOfTimeRangeSpecs, orig.NumberOfTimeRangeSpecs)
	}
	if len(tmpl.TimeRangeSpecs) != len(orig.TimeRangeSpecs) {
		t.Fatalf("len(TimeRangeSpecs) = %d, want %d", len(tmpl.TimeRangeSpecs), len(orig.TimeRangeSpecs))
	}
	for i, tr := range tmpl.TimeRangeSpecs {
		if tr != orig.TimeRangeSpecs[i] {
			t.Errorf("TimeRangeSpecs[%d] mismatch:\n  got  %+v\n  want %+v", i, tr, orig.TimeRangeSpecs[i])
		}
	}
}

func TestWriteSection4_Template412_ByteExact(t *testing.T) {
	raw, err := os.ReadFile("testdata/template412.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	s4 := msg.Fields[0].Section4
	var buf bytes.Buffer
	if err := WriteSection4(&buf, s4); err != nil {
		t.Fatalf("WriteSection4: %v", err)
	}

	got := buf.Bytes()
	s4Offset := findSectionOffset(raw, 4)
	if s4Offset < 0 {
		t.Fatal("could not find section 4 in raw data")
	}
	s4Len := int(readUint32(raw, s4Offset))
	want := raw[s4Offset : s4Offset+s4Len]
	if !bytes.Equal(got, want) {
		t.Errorf("byte mismatch:\n  got  %x\n  want %x", got, want)
	}
}

// findSectionOffset finds the offset of a given section number in raw GRIB2 data.
// It scans through the sections sequentially starting after the indicator section.
func findSectionOffset(raw []byte, sectionNum int) int {
	// Section 0 (Indicator) is always 16 bytes
	off := 16
	for off < len(raw)-5 {
		secLen := int(readUint32(raw, off))
		secNo := int(raw[off+4])
		if secNo == sectionNum {
			return off
		}
		if secLen < 5 {
			break
		}
		off += secLen
	}
	return -1
}
