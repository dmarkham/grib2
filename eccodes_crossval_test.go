package grib2

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// eccodesAvailable returns true if the eccodes tools are installed in the
// reference directory and the ECCODES_DEFINITION_PATH is usable.
func eccodesAvailable(t *testing.T) bool {
	t.Helper()
	gribGet := filepath.Join("reference", "eccodes-install", "bin", "grib_get")
	if _, err := os.Stat(gribGet); err != nil {
		return false
	}
	return true
}

// eccodesEnv returns the environment variables needed to run eccodes tools.
func eccodesEnv() []string {
	base, _ := filepath.Abs("reference/eccodes-install")
	return append(os.Environ(),
		"PATH="+filepath.Join(base, "bin")+":"+os.Getenv("PATH"),
		"ECCODES_DEFINITION_PATH="+filepath.Join(base, "share", "eccodes", "definitions"),
	)
}

// runGribGet runs grib_get with the given flags on a file and returns stdout lines.
func runGribGet(t *testing.T, file string, flags ...string) []string {
	t.Helper()
	gribGet := filepath.Join("reference", "eccodes-install", "bin", "grib_get")
	args := append(flags, file)
	cmd := exec.Command(gribGet, args...)
	cmd.Env = eccodesEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("grib_get %v: %v", args, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines
}

// runGribGetDataFull runs grib_get_data on a specific message and returns
// ALL lat/lon/value triples (no head/tail sampling).
func runGribGetDataFull(t *testing.T, file string, msgIndex int) (lats, lons, vals []float64) {
	t.Helper()
	gribGetData := filepath.Join("reference", "eccodes-install", "bin", "grib_get_data")
	args := []string{"-w", fmt.Sprintf("count=%d", msgIndex), file}
	cmd := exec.Command(gribGetData, args...)
	cmd.Env = eccodesEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("grib_get_data %v: %v", args, err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Scan() // skip header

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		lat, _ := strconv.ParseFloat(fields[0], 64)
		lon, _ := strconv.ParseFloat(fields[1], 64)
		val, _ := strconv.ParseFloat(fields[2], 64)
		lats = append(lats, lat)
		lons = append(lons, lon)
		vals = append(vals, val)
	}
	return
}

// eccodesMeta holds parsed eccodes metadata for a single GRIB message.
type eccodesMeta struct {
	Centre                          string
	GridType                        string
	PackingType                     string
	TypeOfFirstFixedSurface         string
	ScaleFactorOfFirstFixedSurface  int
	ScaledValueOfFirstFixedSurface  int
	Level                           int
	NumberOfValues                  int
}

// parseMetadataLine parses one line of "grib_get -p centre,gridType,..."
func parseMetadataLine(line string) (eccodesMeta, error) {
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return eccodesMeta{}, fmt.Errorf("metadata line has %d fields, want 8: %q", len(fields), line)
	}
	sf, err := strconv.Atoi(fields[4])
	if err != nil {
		return eccodesMeta{}, fmt.Errorf("parsing scaleFactorOfFirstFixedSurface %q: %w", fields[4], err)
	}
	sv, err := strconv.Atoi(fields[5])
	if err != nil {
		return eccodesMeta{}, fmt.Errorf("parsing scaledValueOfFirstFixedSurface %q: %w", fields[5], err)
	}
	lv, err := strconv.Atoi(fields[6])
	if err != nil {
		return eccodesMeta{}, fmt.Errorf("parsing level %q: %w", fields[6], err)
	}
	nv, err := strconv.Atoi(fields[7])
	if err != nil {
		return eccodesMeta{}, fmt.Errorf("parsing numberOfValues %q: %w", fields[7], err)
	}
	return eccodesMeta{
		Centre:                          fields[0],
		GridType:                        fields[1],
		PackingType:                     fields[2],
		TypeOfFirstFixedSurface:         fields[3],
		ScaleFactorOfFirstFixedSurface:  sf,
		ScaledValueOfFirstFixedSurface:  sv,
		Level:                           lv,
		NumberOfValues:                  nv,
	}, nil
}

// ---------------------------------------------------------------------------
// Eccodes cross-validation: field metadata
// ---------------------------------------------------------------------------

// TestEccodes_FieldMetadata compares parsed metadata from our library against
// eccodes reference output for every test fixture that has a .metadata file.
func TestEccodes_FieldMetadata(t *testing.T) {
	if !eccodesAvailable(t) {
		t.Skip("eccodes tools not installed in reference/eccodes-install")
	}

	fixtures := []struct {
		gribFile string
		metaFile string // pre-generated, or "" to run eccodes live
	}{
		{"testdata/rdps_surface.grib2", "testdata/reference/rdps_surface.metadata"},
		{"testdata/rdps_isobaric_850.grib2", "testdata/reference/rdps_isobaric_850.metadata"},
		{"testdata/rdps_isobaric_200.grib2", "testdata/reference/rdps_isobaric_200.metadata"},
		{"testdata/gfs_multifield.grib2", "testdata/reference/gfs_multifield.metadata"},
	}

	for _, fx := range fixtures {
		t.Run(filepath.Base(fx.gribFile), func(t *testing.T) {
			if _, err := os.Stat(fx.gribFile); err != nil {
				t.Skipf("fixture not available: %v", err)
			}

			// Parse with our library
			f, err := os.Open(fx.gribFile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()

			dec := NewDecoder(f)
			var fields []Field
			for {
				msg, err := dec.Decode()
				if err != nil {
					break
				}
				fields = append(fields, msg.Fields...)
			}
			if len(fields) == 0 {
				t.Fatal("no fields parsed from GRIB file")
			}

			// Get eccodes metadata (live)
			metaLines := runGribGet(t, fx.gribFile,
				"-p", "centre,gridType,packingType,typeOfFirstFixedSurface,scaleFactorOfFirstFixedSurface,scaledValueOfFirstFixedSurface,level,numberOfValues")

			if len(metaLines) != len(fields) {
				t.Fatalf("message count mismatch: eccodes=%d, ours=%d", len(metaLines), len(fields))
			}

			for i, line := range metaLines {
				meta, err := parseMetadataLine(line)
				if err != nil {
					t.Fatalf("msg %d: %v", i+1, err)
				}

				field := fields[i]
				tmpl := field.extractTemplate40()
				if tmpl == nil {
					t.Errorf("msg %d: no Template40 found", i+1)
					continue
				}

				// Check scaleFactorOfFirstFixedSurface (the sign-magnitude bug)
				if int(tmpl.ScaleFactorOfFirstSurface) != meta.ScaleFactorOfFirstFixedSurface {
					t.Errorf("msg %d: ScaleFactorOfFirstSurface = %d, eccodes = %d",
						i+1, tmpl.ScaleFactorOfFirstSurface, meta.ScaleFactorOfFirstFixedSurface)
				}

				// Check scaledValueOfFirstFixedSurface
				if int(tmpl.ScaledValueOfFirstSurface) != meta.ScaledValueOfFirstFixedSurface {
					t.Errorf("msg %d: ScaledValueOfFirstSurface = %d, eccodes = %d",
						i+1, tmpl.ScaledValueOfFirstSurface, meta.ScaledValueOfFirstFixedSurface)
				}

				// Check level (SurfaceLevelHPa for isobaric, raw otherwise)
				hpa, err := field.SurfaceLevelHPa()
				if err != nil {
					t.Errorf("msg %d: SurfaceLevelHPa: %v", i+1, err)
					continue
				}
				if math.Abs(hpa-float64(meta.Level)) > 0.5 {
					t.Errorf("msg %d: SurfaceLevelHPa = %f, eccodes level = %d",
						i+1, hpa, meta.Level)
				}

				// Check numberOfValues
				if int(field.Section5.NumberOfValues) != meta.NumberOfValues {
					t.Errorf("msg %d: NumberOfValues = %d, eccodes = %d",
						i+1, field.Section5.NumberOfValues, meta.NumberOfValues)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Eccodes cross-validation: decoded values
// ---------------------------------------------------------------------------

// runGribGetAllValues runs grib_get_data for a specific message and returns
// ALL values (not just head/tail). Used for statistical comparison.
func runGribGetAllValues(t *testing.T, file string, msgIndex int) []float64 {
	t.Helper()
	gribGetData := filepath.Join("reference", "eccodes-install", "bin", "grib_get_data")
	args := []string{"-w", fmt.Sprintf("count=%d", msgIndex), file}
	cmd := exec.Command(gribGetData, args...)
	cmd.Env = eccodesEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("grib_get_data %v: %v", args, err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	// Skip header
	scanner.Scan()

	var vals []float64
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		val, _ := strconv.ParseFloat(fields[2], 64)
		vals = append(vals, val)
	}
	return vals
}

// sortedCopy returns a sorted copy of the float64 slice, excluding NaN values.
func sortedCopy(vals []float64) []float64 {
	out := make([]float64, 0, len(vals))
	for _, v := range vals {
		if !math.IsNaN(v) {
			out = append(out, v)
		}
	}
	sort.Float64s(out)
	return out
}

// TestEccodes_DecodedValues compares our decoded data values against eccodes
// grib_get_data output for each test fixture using order-independent comparison.
//
// Note: eccodes grib_get_data outputs values in geographic lat/lon order,
// while our Values() outputs them in canonical scanning order (which may differ
// for rotated grids). So we compare sorted value distributions rather than
// positional values.
func TestEccodes_DecodedValues(t *testing.T) {
	if !eccodesAvailable(t) {
		t.Skip("eccodes tools not installed in reference/eccodes-install")
	}

	fixtures := []struct {
		gribFile  string
		tolerance float64 // absolute tolerance for value comparison
	}{
		// JPEG2000 is lossy-ish but these should still be very close
		{"testdata/rdps_surface.grib2", 1e-3},
		{"testdata/rdps_isobaric_850.grib2", 1e-3},
		{"testdata/rdps_isobaric_200.grib2", 1e-3},
		// Simple packing: lossless within floating point
		{"testdata/gfs_multifield.grib2", 1e-4},
	}

	for _, fx := range fixtures {
		t.Run(filepath.Base(fx.gribFile), func(t *testing.T) {
			if _, err := os.Stat(fx.gribFile); err != nil {
				t.Skipf("fixture not available: %v", err)
			}

			f, err := os.Open(fx.gribFile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()

			dec := NewDecoder(f)
			msgIdx := 0
			for {
				msg, err := dec.Decode()
				if err != nil {
					break
				}
				for _, field := range msg.Fields {
					msgIdx++
					t.Run(fmt.Sprintf("msg%d", msgIdx), func(t *testing.T) {
						values, err := field.Values()
						if err != nil {
							t.Fatalf("Values(): %v", err)
						}

						// Get ALL eccodes reference values for this message
						refVals := runGribGetAllValues(t, fx.gribFile, msgIdx)

						if len(values) != len(refVals) {
							t.Fatalf("value count mismatch: ours=%d, eccodes=%d", len(values), len(refVals))
						}

						// Sort both and compare element-by-element.
						// This is order-independent: if the same set of values
						// was decoded, they'll match when sorted.
						ourSorted := sortedCopy(values)
						ecSorted := sortedCopy(refVals)

						if len(ourSorted) != len(ecSorted) {
							t.Fatalf("non-NaN count mismatch: ours=%d, eccodes=%d", len(ourSorted), len(ecSorted))
						}

						maxDiff := 0.0
						maxDiffIdx := 0
						mismatches := 0
						for j := range ourSorted {
							diff := math.Abs(ourSorted[j] - ecSorted[j])
							if diff > maxDiff {
								maxDiff = diff
								maxDiffIdx = j
							}
							if diff > fx.tolerance {
								mismatches++
							}
						}

						if mismatches > 0 {
							t.Errorf("%d/%d sorted values differ by more than %e; max diff=%e at sorted index %d (ours=%e, eccodes=%e)",
								mismatches, len(ourSorted), fx.tolerance, maxDiff, maxDiffIdx,
								ourSorted[maxDiffIdx], ecSorted[maxDiffIdx])
						}

						// Also compare statistical properties
						ourMin, ourMax, ourMean := stats(values)
						ecMin, ecMax, ecMean := stats(refVals)

						if math.Abs(ourMin-ecMin) > fx.tolerance {
							t.Errorf("min: ours=%e, eccodes=%e", ourMin, ecMin)
						}
						if math.Abs(ourMax-ecMax) > fx.tolerance {
							t.Errorf("max: ours=%e, eccodes=%e", ourMax, ecMax)
						}
						if math.Abs(ourMean-ecMean) > fx.tolerance {
							t.Errorf("mean: ours=%e, eccodes=%e", ourMean, ecMean)
						}
					})
				}
			}
		})
	}
}

// stats returns min, max, mean of non-NaN values.
func stats(vals []float64) (min, max, mean float64) {
	min = math.Inf(1)
	max = math.Inf(-1)
	sum := 0.0
	count := 0
	for _, v := range vals {
		if math.IsNaN(v) {
			continue
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
		count++
	}
	if count > 0 {
		mean = sum / float64(count)
	}
	return
}

// ---------------------------------------------------------------------------
// Specific regression tests for known bugs
// ---------------------------------------------------------------------------

// TestEccodes_RDPSScaleFactor is the EXACT test that would have caught the
// sign-magnitude encoding bug. It verifies that RDPS files with non-standard
// scale factors (like byte 0x83 = sign-magnitude -3) are parsed correctly.
func TestEccodes_RDPSScaleFactor(t *testing.T) {
	tests := []struct {
		file            string
		wantScaleFactor int8
		wantScaledValue uint32
		wantLevelHPa    float64
	}{
		{
			file:            "testdata/rdps_isobaric_850.grib2",
			wantScaleFactor: -3,
			wantScaledValue: 85,
			wantLevelHPa:    850,
		},
		{
			file:            "testdata/rdps_isobaric_200.grib2",
			wantScaleFactor: -4,
			wantScaledValue: 2,
			wantLevelHPa:    200,
		},
	}

	for _, tt := range tests {
		t.Run(filepath.Base(tt.file), func(t *testing.T) {
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
				t.Fatal("no fields in message")
			}

			field := &msg.Fields[0]
			tmpl := field.extractTemplate40()
			if tmpl == nil {
				t.Fatal("no Template40 in field")
			}

			// Verify sign-magnitude decoding of scale factor
			if tmpl.ScaleFactorOfFirstSurface != tt.wantScaleFactor {
				t.Errorf("ScaleFactorOfFirstSurface = %d, want %d (sign-magnitude bug?)",
					tmpl.ScaleFactorOfFirstSurface, tt.wantScaleFactor)
			}

			if tmpl.ScaledValueOfFirstSurface != tt.wantScaledValue {
				t.Errorf("ScaledValueOfFirstSurface = %d, want %d",
					tmpl.ScaledValueOfFirstSurface, tt.wantScaledValue)
			}

			// Verify the computed level in hPa
			hpa, err := field.SurfaceLevelHPa()
			if err != nil {
				t.Fatalf("SurfaceLevelHPa: %v", err)
			}
			if math.Abs(hpa-tt.wantLevelHPa) > 0.01 {
				t.Errorf("SurfaceLevelHPa() = %f, want %f", hpa, tt.wantLevelHPa)
			}

			// Cross-validate with eccodes if available
			if eccodesAvailable(t) {
				lines := runGribGet(t, tt.file,
					"-p", "scaleFactorOfFirstFixedSurface,scaledValueOfFirstFixedSurface,level")
				if len(lines) > 0 {
					fields := strings.Fields(lines[0])
					if len(fields) >= 3 {
						ecSF, _ := strconv.Atoi(fields[0])
						ecSV, _ := strconv.Atoi(fields[1])
						ecLevel, _ := strconv.Atoi(fields[2])
						if int(tmpl.ScaleFactorOfFirstSurface) != ecSF {
							t.Errorf("scaleFactor: ours=%d, eccodes=%d", tmpl.ScaleFactorOfFirstSurface, ecSF)
						}
						if int(tmpl.ScaledValueOfFirstSurface) != ecSV {
							t.Errorf("scaledValue: ours=%d, eccodes=%d", tmpl.ScaledValueOfFirstSurface, ecSV)
						}
						if math.Abs(hpa-float64(ecLevel)) > 0.5 {
							t.Errorf("level: ours=%f hPa, eccodes=%d hPa", hpa, ecLevel)
						}
					}
				}
			}
		})
	}
}

// TestEccodes_GFSScaleFactor verifies that standard GFS-style scale factors
// (scaleFactor=0, scaledValue=85000 for 850hPa) are also handled correctly.
func TestEccodes_GFSScaleFactor(t *testing.T) {
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

	// Collect all fields and verify isobaric ones have correct levels
	var fields []Field
	for {
		msg, err := dec.Decode()
		if err != nil {
			break
		}
		fields = append(fields, msg.Fields...)
	}

	if len(fields) != 40 {
		t.Fatalf("expected 40 fields in GFS multifield, got %d", len(fields))
	}

	// Cross-validate every field's level with eccodes
	if !eccodesAvailable(t) {
		t.Skip("eccodes tools not available for cross-validation")
	}

	metaLines := runGribGet(t, file,
		"-p", "level,typeOfFirstFixedSurface,scaleFactorOfFirstFixedSurface,scaledValueOfFirstFixedSurface")

	for i, line := range metaLines {
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		ecLevel, _ := strconv.Atoi(parts[0])
		ecSF, _ := strconv.Atoi(parts[2])
		ecSV, _ := strconv.Atoi(parts[3])

		field := fields[i]
		tmpl := field.extractTemplate40()
		if tmpl == nil {
			continue
		}

		if int(tmpl.ScaleFactorOfFirstSurface) != ecSF {
			t.Errorf("field %d: ScaleFactorOfFirstSurface = %d, eccodes = %d",
				i+1, tmpl.ScaleFactorOfFirstSurface, ecSF)
		}
		if int(tmpl.ScaledValueOfFirstSurface) != ecSV {
			t.Errorf("field %d: ScaledValueOfFirstSurface = %d, eccodes = %d",
				i+1, tmpl.ScaledValueOfFirstSurface, ecSV)
		}

		hpa, err := field.SurfaceLevelHPa()
		if err != nil {
			t.Errorf("field %d: SurfaceLevelHPa: %v", i+1, err)
			continue
		}
		if math.Abs(hpa-float64(ecLevel)) > 0.5 {
			t.Errorf("field %d: SurfaceLevelHPa = %f, eccodes level = %d",
				i+1, hpa, ecLevel)
		}
	}
}

// TestEccodes_ValueCount verifies that our decoded value count matches eccodes
// numberOfValues for every test fixture.
func TestEccodes_ValueCount(t *testing.T) {
	if !eccodesAvailable(t) {
		t.Skip("eccodes tools not installed")
	}

	fixtures := []string{
		"testdata/rdps_surface.grib2",
		"testdata/rdps_isobaric_850.grib2",
		"testdata/rdps_isobaric_200.grib2",
		"testdata/gfs_multifield.grib2",
	}

	for _, fx := range fixtures {
		t.Run(filepath.Base(fx), func(t *testing.T) {
			if _, err := os.Stat(fx); err != nil {
				t.Skipf("fixture not available: %v", err)
			}

			f, err := os.Open(fx)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()

			dec := NewDecoder(f)
			metaLines := runGribGet(t, fx, "-p", "numberOfValues")
			msgIdx := 0
			for {
				msg, err := dec.Decode()
				if err != nil {
					break
				}
				for _, field := range msg.Fields {
					values, err := field.Values()
					if err != nil {
						t.Errorf("msg %d: Values(): %v", msgIdx+1, err)
						msgIdx++
						continue
					}

					if msgIdx < len(metaLines) {
						ecNV, _ := strconv.Atoi(strings.TrimSpace(metaLines[msgIdx]))
						// For fields with bitmaps, eccodes numberOfValues is the
						// number of data points (including missing); our Values()
						// returns the same count after bitmap expansion.
						if len(values) != ecNV && int(field.Section3.NumberOfDataPoints) != ecNV {
							t.Errorf("msg %d: len(Values()) = %d, Section3.NumberOfDataPoints = %d, eccodes numberOfValues = %d",
								msgIdx+1, len(values), field.Section3.NumberOfDataPoints, ecNV)
						}
					}
					msgIdx++
				}
			}
		})
	}
}
