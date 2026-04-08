package grib2

import (
	"bytes"
	"math"
	"os"
	"testing"
)

func TestUnpackRunLength_SampleFile(t *testing.T) {
	raw, err := os.ReadFile("testdata/run_length_packing.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	tmpl, ok := field.Section5.Template.(Template5200)
	if !ok {
		t.Fatalf("not Template5200: %T", field.Section5.Template)
	}

	t.Logf("Template5200: bitsPerValue=%d, maxLevelValue=%d, numberOfLevelValues=%d, decimalScaleFactor=%d",
		tmpl.BitsPerValue, tmpl.MaxLevelValue, tmpl.NumberOfLevelValues, tmpl.DecimalScaleFactor)
	t.Logf("LevelValues: %v", tmpl.LevelValues)
	t.Logf("NumberOfValues: %d, Section7 data length: %d", field.Section5.NumberOfValues, len(field.Section7.Data))

	const missingValue = 9999.0
	values, err := UnpackRunLength(field.Section7.Data, tmpl, field.Section5.NumberOfValues, missingValue)
	if err != nil {
		t.Fatalf("UnpackRunLength: %v", err)
	}

	if uint32(len(values)) != field.Section5.NumberOfValues {
		t.Fatalf("got %d values, want %d", len(values), field.Section5.NumberOfValues)
	}

	// Load eccodes reference values
	refValues := loadReferenceValues(t, "testdata/reference/run_length_packing.grib2.values")
	if len(refValues) != len(values) {
		t.Fatalf("reference has %d values, got %d", len(refValues), len(values))
	}

	// Compare with eccodes reference
	maxErr := 0.0
	errCount := 0
	for i, got := range values {
		want := refValues[i]
		diff := math.Abs(got - want)
		if diff > maxErr {
			maxErr = diff
		}
		if diff > 1e-6 {
			errCount++
			if errCount <= 10 {
				t.Errorf("value[%d] = %.10e, want %.10e (diff = %.10e)", i, got, want, diff)
			}
		}
	}
	if errCount > 10 {
		t.Errorf("... and %d more mismatches", errCount-10)
	}
	t.Logf("max error vs eccodes: %.2e (across %d values)", maxErr, len(values))

	// Verify we get the expected distinct values: 1, 2, 3 and 9999 (missing)
	seen := map[float64]int{}
	for _, v := range values {
		seen[v]++
	}
	t.Logf("distinct values: %v", seen)

	// eccodes grib_get_data only outputs non-missing values (1, 2, 3)
	// but the data also has missing values (level 0 -> 9999)
	for _, expected := range []float64{1.0, 2.0, 3.0} {
		if seen[expected] == 0 {
			t.Errorf("expected value %.0f not found in decoded data", expected)
		}
	}
}

func TestTemplate48_ReadWrite(t *testing.T) {
	raw, err := os.ReadFile("testdata/template48.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	field := msg.Fields[0]
	if field.Section4.TemplateNumber != 8 {
		t.Fatalf("S4.TemplateNumber = %d, want 8", field.Section4.TemplateNumber)
	}

	tmpl, ok := field.Section4.Template.(Template48)
	if !ok {
		t.Fatalf("S4.Template type = %T, want Template48", field.Section4.Template)
	}

	// Verify Template 4.0 base fields
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

	// Verify Template 4.8 extension fields
	if tmpl.YearOfEndOfInterval != 2021 {
		t.Errorf("YearOfEndOfInterval = %d, want 2021", tmpl.YearOfEndOfInterval)
	}
	if tmpl.MonthOfEndOfInterval != 8 {
		t.Errorf("MonthOfEndOfInterval = %d, want 8", tmpl.MonthOfEndOfInterval)
	}
	if tmpl.DayOfEndOfInterval != 18 {
		t.Errorf("DayOfEndOfInterval = %d, want 18", tmpl.DayOfEndOfInterval)
	}
	if tmpl.HourOfEndOfInterval != 0 {
		t.Errorf("HourOfEndOfInterval = %d, want 0", tmpl.HourOfEndOfInterval)
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
	if tr.TypeOfStatisticalProcessing != 1 {
		t.Errorf("TypeOfStatisticalProcessing = %d, want 1 (Accumulation)", tr.TypeOfStatisticalProcessing)
	}
	if tr.TypeOfTimeIncrement != 2 {
		t.Errorf("TypeOfTimeIncrement = %d, want 2", tr.TypeOfTimeIncrement)
	}
	if tr.IndicatorOfUnitForTimeRange != 1 {
		t.Errorf("IndicatorOfUnitForTimeRange = %d, want 1 (Hour)", tr.IndicatorOfUnitForTimeRange)
	}
	if tr.LengthOfTimeRange != 6 {
		t.Errorf("LengthOfTimeRange = %d, want 6", tr.LengthOfTimeRange)
	}
	if tr.IndicatorOfUnitForTimeIncrement != 1 {
		t.Errorf("IndicatorOfUnitForTimeIncrement = %d, want 1 (Hour)", tr.IndicatorOfUnitForTimeIncrement)
	}
	if tr.TimeIncrement != 0 {
		t.Errorf("TimeIncrement = %d, want 0", tr.TimeIncrement)
	}

	// Write back and verify byte-exact round-trip
	var buf bytes.Buffer
	if err := WriteMessage(&buf, msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	got := buf.Bytes()
	if !bytes.Equal(got, raw) {
		// Find first differing byte
		for i := range got {
			if i >= len(raw) || got[i] != raw[i] {
				t.Fatalf("first diff at byte %d: got 0x%02x, want 0x%02x", i, got[i], raw[i])
			}
		}
		if len(got) != len(raw) {
			t.Fatalf("length mismatch: got %d, want %d", len(got), len(raw))
		}
	}
	t.Log("byte-exact round-trip OK")
}
