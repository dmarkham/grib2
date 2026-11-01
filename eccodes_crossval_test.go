package grib2

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// eccodesAvailable returns true if the eccodes tools are installed in the
// reference directory and the ECCODES_DEFINITION_PATH is usable.
func eccodesAvailable(t *testing.T) bool {
	t.Helper()
	if os.Getenv("GRIB2_NO_ECCODES") != "" {
		return false // run as if eccodes were not installed
	}
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

// ---------------------------------------------------------------------------
// Eccodes cross-validation: field metadata
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Eccodes cross-validation: decoded values
// ---------------------------------------------------------------------------

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
func TestRDPSScaleFactor(t *testing.T) {
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

		})
	}
}


// execEccodes returns a command running an eccodes tool from reference/.
func execEccodes(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(filepath.Join("reference", "eccodes-install", "bin", name), args...)
	cmd.Env = eccodesEnv()
	return cmd
}
