package grib2

import (
	"math"
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// Integration test: RDPS surface temperature near San Diego
// ---------------------------------------------------------------------------

func TestIntegration_RDPSSanDiego(t *testing.T) {
	file := "testdata/rdps_surface.grib2"
	if _, err := os.Stat(file); err != nil {
		t.Skipf("fixture not available: %v", err)
	}

	f, err := os.Open(file)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	msg, err := ReadMessage(f)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if len(msg.Fields) == 0 {
		t.Fatal("no fields")
	}

	field := &msg.Fields[0]

	// San Diego, CA
	val, idx, nearLat, nearLon, err := field.NearestValue(32.715, -117.161)
	if err != nil {
		t.Fatalf("NearestValue: %v", err)
	}

	t.Logf("NearestValue(32.715, -117.161) = %.2f K at index %d (lat=%.3f, lon=%.3f)", val, idx, nearLat, nearLon)

	// Temperature should be physically reasonable (200-330 K covers all weather)
	if val < 200 || val > 330 {
		t.Errorf("temperature %.2f K is outside physically reasonable range [200, 330]", val)
	}

	// The RDPS surface file is "AGL-2m" temperature, typeOfFirstFixedSurface=103
	surfType, err := field.SurfaceType()
	if err != nil {
		t.Fatalf("SurfaceType: %v", err)
	}
	// 103 = "Specified height level above ground", or 1 = "Ground or Water Surface"
	// RDPS AGL-2m uses 103 (height above ground)
	if surfType != 103 {
		t.Logf("SurfaceType = %d (expected 103 for height-above-ground)", surfType)
	}

	// Cross-check: brute-force NearestValue should give the same result
	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	// Find nearest by brute force
	bestIdx := 0
	bestDist := math.MaxFloat64
	for i := range lats {
		d := haversineDeg(32.715, -117.161, lats[i], lons[i])
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}

	if bestIdx != idx {
		t.Errorf("brute-force nearest index = %d (val=%.2f), NearestValue index = %d (val=%.2f)",
			bestIdx, values[bestIdx], idx, val)
	}
}

// ---------------------------------------------------------------------------
// Integration test: RDPS isobaric levels
// ---------------------------------------------------------------------------

func TestIntegration_RDPSIsobaricLevels(t *testing.T) {
	tests := []struct {
		file     string
		wantHPa  float64
	}{
		{"testdata/rdps_isobaric_850.grib2", 850},
		{"testdata/rdps_isobaric_200.grib2", 200},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if _, err := os.Stat(tt.file); err != nil {
				t.Skipf("fixture not available: %v", err)
			}

			f, err := os.Open(tt.file)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()

			msg, err := ReadMessage(f)
			if err != nil {
				t.Fatalf("ReadMessage: %v", err)
			}
			if len(msg.Fields) == 0 {
				t.Fatal("no fields")
			}

			field := &msg.Fields[0]

			// Verify surface type is isobaric (100)
			surfType, err := field.SurfaceType()
			if err != nil {
				t.Fatalf("SurfaceType: %v", err)
			}
			if surfType != 100 {
				t.Errorf("SurfaceType = %d, want 100 (isobaric)", surfType)
			}

			// Verify SurfaceLevelHPa matches expected
			hpa, err := field.SurfaceLevelHPa()
			if err != nil {
				t.Fatalf("SurfaceLevelHPa: %v", err)
			}
			if math.Abs(hpa-tt.wantHPa) > 0.01 {
				t.Errorf("SurfaceLevelHPa = %f, want %f", hpa, tt.wantHPa)
			}

			// Also verify SurfaceLevel returns Pa
			pa, err := field.SurfaceLevel()
			if err != nil {
				t.Fatalf("SurfaceLevel: %v", err)
			}
			if math.Abs(pa-tt.wantHPa*100) > 1.0 {
				t.Errorf("SurfaceLevel = %f Pa, want %f Pa", pa, tt.wantHPa*100)
			}

			// Decode values and verify they're physically reasonable temperatures
			values, err := field.Values()
			if err != nil {
				t.Fatalf("Values: %v", err)
			}
			if len(values) == 0 {
				t.Fatal("no values decoded")
			}

			// Check that most values are reasonable temperatures (150-350 K)
			outOfRange := 0
			for _, v := range values {
				if math.IsNaN(v) {
					continue
				}
				if v < 150 || v > 350 {
					outOfRange++
				}
			}
			if pct := float64(outOfRange) / float64(len(values)) * 100; pct > 1 {
				t.Errorf("%.1f%% of values (%d/%d) are outside [150, 350] K range",
					pct, outOfRange, len(values))
			}

			// Cross-validate with eccodes if available
			if eccodesAvailable(t) {
				lines := runGribGet(t, tt.file, "-p", "level")
				if len(lines) > 0 {
					t.Logf("eccodes level: %s", lines[0])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration test: GFS multi-field file
// ---------------------------------------------------------------------------

func TestIntegration_GFSMultiField(t *testing.T) {
	file := "testdata/gfs_multifield.grib2"
	if _, err := os.Stat(file); err != nil {
		t.Skipf("fixture not available: %v", err)
	}

	f, err := os.Open(file)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	dec := NewDecoder(f)
	var allFields []Field
	msgCount := 0
	for {
		msg, err := dec.Decode()
		if err != nil {
			break
		}
		msgCount++
		allFields = append(allFields, msg.Fields...)
	}

	// The GFS file has 40 messages (one per field)
	t.Logf("decoded %d messages, %d total fields", msgCount, len(allFields))

	if len(allFields) != 40 {
		t.Fatalf("expected 40 fields, got %d", len(allFields))
	}

	// Verify each field can be decoded and has the right number of values
	for i, field := range allFields {
		values, err := field.Values()
		if err != nil {
			t.Errorf("field %d: Values(): %v", i+1, err)
			continue
		}
		// GFS file has 117 values per field (13x9 grid)
		if len(values) != 117 {
			t.Errorf("field %d: len(Values()) = %d, want 117", i+1, len(values))
		}
	}

	// Spot-check: field 2 should be temperature at 200 hPa
	field2 := &allFields[1]
	tmpl := field2.extractTemplate40()
	if tmpl == nil {
		t.Fatal("field 2: no Template40")
	}
	// ParameterCategory=0 (Temperature), ParameterNumber=0 (Temperature)
	if tmpl.ParameterCategory != 0 || tmpl.ParameterNumber != 0 {
		t.Errorf("field 2: param (%d,%d), expected (0,0) for temperature",
			tmpl.ParameterCategory, tmpl.ParameterNumber)
	}

	hpa, err := field2.SurfaceLevelHPa()
	if err != nil {
		t.Fatalf("field 2: SurfaceLevelHPa: %v", err)
	}
	if math.Abs(hpa-200) > 0.5 {
		t.Errorf("field 2: SurfaceLevelHPa = %f, want 200", hpa)
	}

	// Spot-check: first field should be surface visibility
	field1 := &allFields[0]
	tmpl1 := field1.extractTemplate40()
	if tmpl1 == nil {
		t.Fatal("field 1: no Template40")
	}
	if tmpl1.TypeOfFirstFixedSurface != 1 {
		t.Errorf("field 1: TypeOfFirstFixedSurface = %d, want 1 (surface)", tmpl1.TypeOfFirstFixedSurface)
	}
}

// ---------------------------------------------------------------------------
// Integration test: RDPS grid coordinates
// ---------------------------------------------------------------------------

func TestIntegration_RDPSGridCoordinates(t *testing.T) {
	file := "testdata/rdps_surface.grib2"
	if _, err := os.Stat(file); err != nil {
		t.Skipf("fixture not available: %v", err)
	}

	f, err := os.Open(file)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	msg, err := ReadMessage(f)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if len(msg.Fields) == 0 {
		t.Fatal("no fields")
	}

	field := &msg.Fields[0]

	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	// Coordinate count must match numberOfDataPoints
	ndp := int(field.Section3.NumberOfDataPoints)
	if len(lats) != ndp {
		t.Errorf("len(lats) = %d, NumberOfDataPoints = %d", len(lats), ndp)
	}
	if len(lons) != ndp {
		t.Errorf("len(lons) = %d, NumberOfDataPoints = %d", len(lons), ndp)
	}

	// Values() count must match coordinates count
	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(values) != len(lats) {
		t.Errorf("len(Values()) = %d, len(GridCoordinates) = %d -- these must match for NearestValue to work",
			len(values), len(lats))
	}

	// RDPS uses rotated lat/lon (Template31). After canonical reorder
	// (north-to-south), the first latitude should be > the last latitude.
	if lats[0] < lats[len(lats)-1] {
		t.Errorf("canonical order: first lat (%.3f) should be >= last lat (%.3f) for N-to-S ordering",
			lats[0], lats[len(lats)-1])
	}

	// All latitudes should be in [-90, 90] range (geographic)
	for i, lat := range lats {
		if lat < -90 || lat > 90 {
			t.Errorf("lat[%d] = %.3f is outside [-90, 90]", i, lat)
			break
		}
	}

	// All longitudes should be in [-360, 360] range
	for i, lon := range lons {
		if lon < -360 || lon > 360 {
			t.Errorf("lon[%d] = %.3f is outside [-360, 360]", i, lon)
			break
		}
	}

	t.Logf("Grid: %d points, lat range [%.3f, %.3f], lon range [%.3f, %.3f]",
		len(lats),
		minFloat(lats), maxFloat(lats),
		minFloat(lons), maxFloat(lons))
}

// ---------------------------------------------------------------------------
// Integration test: GFS grid coordinates
// ---------------------------------------------------------------------------

func TestIntegration_GFSGridCoordinates(t *testing.T) {
	file := "testdata/gfs_multifield.grib2"
	if _, err := os.Stat(file); err != nil {
		t.Skipf("fixture not available: %v", err)
	}

	f, err := os.Open(file)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	msg, err := ReadMessage(f)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if len(msg.Fields) == 0 {
		t.Fatal("no fields")
	}

	field := &msg.Fields[0]

	lats, lons, err := field.GridCoordinates()
	if err != nil {
		t.Fatalf("GridCoordinates: %v", err)
	}

	values, err := field.Values()
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(values) != len(lats) {
		t.Errorf("len(Values()) = %d, len(GridCoordinates) = %d", len(values), len(lats))
	}

	// Cross-validate coordinates against eccodes grib_get_data.
	// Note: eccodes grib_get_data outputs values in the file's scanning order
	// (south-to-north for mode 64), while our GridCoordinates + Values use
	// canonical order (north-to-south). So we match coordinate-value pairs
	// by building a map keyed on approximate (lat,lon).
	if eccodesAvailable(t) {
		refLats, refLons, refVals := runGribGetDataFull(t, file, 1)

		if len(refLats) != len(lats) {
			t.Fatalf("point count: ours=%d, eccodes=%d", len(lats), len(refLats))
		}

		// Build a map from rounded coordinates to eccodes value
		type coordKey struct {
			lat, lon int // in 0.001 degree units
		}
		ecMap := make(map[coordKey]float64, len(refLats))
		for i := range refLats {
			k := coordKey{
				lat: int(math.Round(refLats[i] * 1000)),
				lon: int(math.Round(refLons[i] * 1000)),
			}
			ecMap[k] = refVals[i]
		}

		// Check that every point in our output has a matching eccodes point
		mismatches := 0
		for i := range lats {
			ourLon := lons[i]
			if ourLon < 0 {
				ourLon += 360
			}
			k := coordKey{
				lat: int(math.Round(lats[i] * 1000)),
				lon: int(math.Round(ourLon * 1000)),
			}
			ecVal, ok := ecMap[k]
			if !ok {
				if mismatches < 5 {
					t.Errorf("point %d: our coord (%.3f, %.3f) not found in eccodes output",
						i, lats[i], lons[i])
				}
				mismatches++
				continue
			}
			if math.Abs(values[i]-ecVal) > 1e-4 {
				if mismatches < 5 {
					t.Errorf("point %d at (%.3f, %.3f): ours=%e, eccodes=%e",
						i, lats[i], lons[i], values[i], ecVal)
				}
				mismatches++
			}
		}
		if mismatches > 0 {
			t.Errorf("total coordinate/value mismatches: %d/%d", mismatches, len(lats))
		}
	}
}

// ---------------------------------------------------------------------------
// Integration test: RDPS decode all values are finite
// ---------------------------------------------------------------------------

func TestIntegration_RDPSDecodeComplete(t *testing.T) {
	files := []string{
		"testdata/rdps_surface.grib2",
		"testdata/rdps_isobaric_850.grib2",
		"testdata/rdps_isobaric_200.grib2",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			if _, err := os.Stat(file); err != nil {
				t.Skipf("fixture not available: %v", err)
			}

			f, err := os.Open(file)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()

			msg, err := ReadMessage(f)
			if err != nil {
				t.Fatalf("ReadMessage: %v", err)
			}
			if len(msg.Fields) == 0 {
				t.Fatal("no fields")
			}

			values, err := msg.Fields[0].Values()
			if err != nil {
				t.Fatalf("Values: %v", err)
			}

			// RDPS files should have 1191300 data points
			if len(values) != 1191300 {
				t.Errorf("len(Values()) = %d, want 1191300", len(values))
			}

			// Count NaN and Inf values
			nanCount := 0
			infCount := 0
			for _, v := range values {
				if math.IsNaN(v) {
					nanCount++
				}
				if math.IsInf(v, 0) {
					infCount++
				}
			}

			// No values should be Inf
			if infCount > 0 {
				t.Errorf("%d Inf values found", infCount)
			}

			// Some NaN is OK (bitmap/missing), but most should be valid
			if pct := float64(nanCount) / float64(len(values)) * 100; pct > 10 {
				t.Errorf("%.1f%% NaN values (%d/%d) seems too high", pct, nanCount, len(values))
			}

			t.Logf("decoded %d values, %d NaN, %d Inf", len(values), nanCount, infCount)
		})
	}
}

// ---------------------------------------------------------------------------
// Integration test: round-trip read-write-read
// ---------------------------------------------------------------------------

func TestIntegration_RoundTrip(t *testing.T) {
	files := []string{
		"testdata/gfs_multifield.grib2",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			if _, err := os.Stat(file); err != nil {
				t.Skipf("fixture not available: %v", err)
			}

			// Read all messages
			f, err := os.Open(file)
			if err != nil {
				t.Fatalf("open: %v", err)
			}

			dec := NewDecoder(f)
			var messages []*Message
			for {
				msg, err := dec.Decode()
				if err != nil {
					break
				}
				messages = append(messages, msg)
			}
			f.Close()

			if len(messages) == 0 {
				t.Fatal("no messages read")
			}

			// Write and re-read each message
			for i, msg := range messages {
				// Get original values
				var origValues [][]float64
				for _, field := range msg.Fields {
					vals, err := field.Values()
					if err != nil {
						t.Fatalf("msg %d: original Values(): %v", i+1, err)
					}
					origValues = append(origValues, vals)
				}

				// Write to temp file
				tmpFile, err := os.CreateTemp("", "grib2-roundtrip-*.grib2")
				if err != nil {
					t.Fatalf("create temp: %v", err)
				}
				defer os.Remove(tmpFile.Name())

				if err := WriteMessage(tmpFile, msg); err != nil {
					tmpFile.Close()
					t.Fatalf("msg %d: WriteMessage: %v", i+1, err)
				}
				tmpFile.Close()

				// Re-read
				f2, err := os.Open(tmpFile.Name())
				if err != nil {
					t.Fatalf("reopen: %v", err)
				}
				msg2, err := ReadMessage(f2)
				f2.Close()
				if err != nil {
					t.Fatalf("msg %d: re-read: %v", i+1, err)
				}

				if len(msg2.Fields) != len(msg.Fields) {
					t.Fatalf("msg %d: field count %d vs %d", i+1, len(msg2.Fields), len(msg.Fields))
				}

				for j, field := range msg2.Fields {
					vals, err := field.Values()
					if err != nil {
						t.Fatalf("msg %d field %d: re-read Values(): %v", i+1, j+1, err)
					}
					if len(vals) != len(origValues[j]) {
						t.Errorf("msg %d field %d: value count %d vs %d",
							i+1, j+1, len(vals), len(origValues[j]))
						continue
					}
					for k := range vals {
						if math.IsNaN(vals[k]) && math.IsNaN(origValues[j][k]) {
							continue
						}
						if vals[k] != origValues[j][k] {
							t.Errorf("msg %d field %d val[%d]: %.6e vs %.6e",
								i+1, j+1, k, vals[k], origValues[j][k])
							break
						}
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func minFloat(s []float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	m := s[0]
	for _, v := range s[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxFloat(s []float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	m := s[0]
	for _, v := range s[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
