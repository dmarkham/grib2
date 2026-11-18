package grib2

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// hrrrInventory lists the parameter of every message of a whole HRRR
// surface file, with the names wgrib2 and eccodes give them.
const hrrrInventory = "testdata/reference/hrrr_wrfsfc_inventory.tsv"

// TestParameterNameFor_HRRR names every parameter of a real HRRR surface
// file. 40 of its 170 messages use NCEP's local parameters; ParameterName
// knows none of those, and ParameterNameFor all but one, which NCEP's table
// does not define either. Where eccodes has a name for a local parameter it
// must be ours, apart from three that eccodes words differently.
func TestParameterNameFor_HRRR(t *testing.T) {
	raw, err := os.ReadFile(hrrrInventory)
	if err != nil {
		t.Fatal(err)
	}
	// eccodes' wording, lower case -> NCEP's.
	reworded := map[string]string{
		"maximum/composite radar reflectivity": "composite reflectivity",
		"derived radar reflectivity":           "reflectivity",
		"best (4-layer) lifted index":          "best (4 layer) lifted index",
	}
	messages, local, unknownWMO, unknown, withEccodes := 0, 0, 0, 0, 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 7 {
			t.Fatalf("bad line %q", line)
		}
		num := func(s string) uint8 {
			v, err := strconv.ParseUint(s, 10, 8)
			if err != nil {
				t.Fatalf("bad line %q: %v", line, err)
			}
			return uint8(v)
		}
		d, c, n, abbrev, eccodesName := num(f[1]), num(f[2]), num(f[3]), f[4], f[6]
		messages++
		name := ParameterNameFor(7, d, c, n)
		unit := ParameterUnitFor(7, d, c, n)
		if !localUseParameter(c, n) {
			if name != ParameterName(d, c, n) || unit != ParameterUnit(d, c, n) {
				t.Errorf("message %s (%d, %d, %d): %q [%s] for NCEP, but it is a WMO parameter: %q [%s]",
					f[0], d, c, n, name, unit, ParameterName(d, c, n), ParameterUnit(d, c, n))
			}
			if strings.HasPrefix(name, "Unknown") {
				t.Errorf("message %s %s (%d, %d, %d): WMO parameter without a name", f[0], abbrev, d, c, n)
			}
			continue
		}
		local++
		if strings.HasPrefix(ParameterName(d, c, n), "Unknown") {
			unknownWMO++
		}
		// Another centre's local parameters are not NCEP's.
		if other := ParameterNameFor(54, d, c, n); !strings.HasPrefix(other, "Unknown") {
			t.Errorf("(%d, %d, %d) is named %q for centre 54", d, c, n, other)
		}
		if strings.HasPrefix(name, "Unknown") {
			unknown++
			// wgrib2 has no name for it either: its .idx shows the numbers.
			if !strings.HasPrefix(abbrev, "var discipline=") || eccodesName != "unknown" || unit != "unknown" {
				t.Errorf("message %s %s (%d, %d, %d): no name, eccodes has %q", f[0], abbrev, d, c, n, eccodesName)
			}
			continue
		}
		if unit == "unknown" || unit == "" {
			t.Errorf("message %s %s: %q has no unit", f[0], abbrev, name)
		}
		if eccodesName != "unknown" {
			withEccodes++
			want := strings.ToLower(eccodesName)
			if r, ok := reworded[want]; ok {
				want = r
			}
			if strings.ToLower(name) != want {
				t.Errorf("message %s %s (%d, %d, %d): %q, eccodes %q", f[0], abbrev, d, c, n, name, eccodesName)
			}
		}
	}
	if messages != 170 || local != 40 || unknownWMO != 40 || unknown != 1 || withEccodes != 21 {
		t.Errorf("%d messages, %d with local parameters (%d unknown to ParameterName, %d to ParameterNameFor, %d named by eccodes); want 170, 40, 40, 1, 21",
			messages, local, unknownWMO, unknown, withEccodes)
	}
}

// TestParameterNameFor_NCEP pins well-known NCEP parameters to the names
// and units in NCEP's GRIB2 documentation (Table 4.2-0-16, 4.2-0-17,
// 4.2-0-1), and the edges of the local-use ranges.
func TestParameterNameFor_NCEP(t *testing.T) {
	for _, c := range []struct {
		d, c, n    uint8
		name, unit string
	}{
		{0, 16, 195, "Reflectivity", "dB"},
		{0, 16, 196, "Composite reflectivity", "dB"},
		{0, 16, 198, "Hourly Maximum of Simulated Reflectivity", "dB"},
		{0, 17, 192, "Lightning", "non-dim"},
		{0, 1, 192, "Categorical Rain", "-"},
		{0, 1, 193, "Categorical Freezing Rain", "-"},
		{0, 1, 194, "Categorical Ice Pellets", "-"},
		{0, 1, 195, "Categorical Snow", "-"},
		{0, 7, 199, "Hourly Maximum of Updraft Helicity", "m^2/s^2"},
		{0, 3, 198, "MSLP (MAPS System Reduction)", "Pa"},
		{3, 192, 1, "Simulated Brightness Temperature for GOES 12, Channel 3", "K"},
	} {
		if got := ParameterNameFor(7, c.d, c.c, c.n); got != c.name {
			t.Errorf("ParameterNameFor(7, %d, %d, %d) = %q, want %q", c.d, c.c, c.n, got, c.name)
		}
		if got := ParameterUnitFor(7, c.d, c.c, c.n); got != c.unit {
			t.Errorf("ParameterUnitFor(7, %d, %d, %d) = %q, want %q", c.d, c.c, c.n, got, c.unit)
		}
		if got := ParameterName(c.d, c.c, c.n); !strings.HasPrefix(got, "Unknown parameter") {
			t.Errorf("ParameterName(%d, %d, %d) = %q: a local parameter has a name without a centre", c.d, c.c, c.n, got)
		}
	}
	// WMO parameters are the same for every centre.
	for _, centre := range []uint16{7, 54, 98, 0xFFFF} {
		if got := ParameterNameFor(centre, 0, 0, 0); got != "Temperature" {
			t.Errorf("ParameterNameFor(%d, 0, 0, 0) = %q", centre, got)
		}
		if got := ParameterUnitFor(centre, 0, 0, 0); got != "K" {
			t.Errorf("ParameterUnitFor(%d, 0, 0, 0) = %q", centre, got)
		}
	}
	// 255 is "missing", not a local code.
	if localUseParameter(255, 0) || localUseParameter(0, 255) || !localUseParameter(192, 0) || !localUseParameter(0, 254) || localUseParameter(191, 191) {
		t.Error("local-use range is not 192-254")
	}
	// Every generated entry is in the local-use range and has a name.
	if len(ncepLocalParameterTable) < 450 || len(ncepLocalParameterTable) != len(ncepLocalParameterUnitTable) {
		t.Errorf("%d names, %d units", len(ncepLocalParameterTable), len(ncepLocalParameterUnitTable))
	}
	for key, name := range ncepLocalParameterTable {
		if !localUseParameter(uint8(key>>8), uint8(key)) || name == "" {
			t.Errorf("entry %#x %q is not a local-use parameter with a name", key, name)
		}
		if _, ok := ncepLocalParameterUnitTable[key]; !ok {
			t.Errorf("entry %#x %q has no unit", key, name)
		}
	}
}
