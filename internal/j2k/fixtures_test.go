package j2k

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// j2kFixture is one line of testdata/manifest.txt.
type j2kFixture struct {
	name   string
	w, h   int
	prec   int
	signed bool
	truth  string // raw ground-truth file, "-" when an error is expected
	expect string // "exact" or "error:<substring>"
	args   string // opj_compress arguments (informational)
}

func loadManifest(t *testing.T) []j2kFixture {
	t.Helper()
	f, err := os.Open("testdata/manifest.txt")
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer f.Close()
	var out []j2kFixture
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		args := ""
		if i := strings.Index(line, "#"); i >= 0 {
			args = strings.TrimSpace(line[i+1:])
			line = line[:i]
		}
		fs := strings.Fields(line)
		if len(fs) != 7 {
			t.Fatalf("bad manifest line %q", line)
		}
		w, _ := strconv.Atoi(fs[1])
		h, _ := strconv.Atoi(fs[2])
		p, _ := strconv.Atoi(fs[3])
		out = append(out, j2kFixture{
			name: fs[0], w: w, h: h, prec: p, signed: fs[4] == "s",
			truth: fs[5], expect: fs[6], args: args,
		})
	}
	if len(out) == 0 {
		t.Fatal("empty manifest")
	}
	return out
}

// readRaw reads an opj-style raw file: 1 byte per sample for prec <= 8,
// otherwise 2 bytes big-endian.
func readRaw(t *testing.T, path string, n, prec int, signed bool) []int32 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	bps := 1
	if prec > 8 {
		bps = 2
	}
	if len(data) != n*bps {
		t.Fatalf("%s: %d bytes, want %d", path, len(data), n*bps)
	}
	out := make([]int32, n)
	for i := range out {
		switch {
		case bps == 1 && signed:
			out[i] = int32(int8(data[i]))
		case bps == 1:
			out[i] = int32(data[i])
		case signed:
			out[i] = int32(int16(binary.BigEndian.Uint16(data[2*i:])))
		default:
			out[i] = int32(binary.BigEndian.Uint16(data[2*i:]))
		}
	}
	return out
}

func compareSamples(t *testing.T, got, want []int32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d samples, want %d", len(got), len(want))
	}
	bad := 0
	for i := range want {
		if got[i] != want[i] {
			if bad < 5 {
				t.Errorf("sample[%d] = %d, want %d", i, got[i], want[i])
			}
			bad++
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d samples differ", bad, len(want))
	}
}

// TestDecodeOpenJPEGFixtures decodes every opj_compress codestream in
// testdata and requires a sample-exact match with the opj_decompress
// output (or the lossless source), or the documented error. A silent
// mis-decode is never acceptable (audit M23, L30, H20).
func TestDecodeOpenJPEGFixtures(t *testing.T) {
	for _, fx := range loadManifest(t) {
		t.Run(fx.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", fx.name+".j2k"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			got, w, h, err := Decode(data)
			if strings.HasPrefix(fx.expect, "error:") {
				want := strings.TrimPrefix(fx.expect, "error:")
				if err == nil {
					t.Fatalf("Decode succeeded (%dx%d), want error containing %q", w, h, want)
				}
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode (%s): %v", fx.args, err)
			}
			if w != fx.w || h != fx.h {
				t.Fatalf("dimensions %dx%d, want %dx%d", w, h, fx.w, fx.h)
			}
			want := readRaw(t, filepath.Join("testdata", fx.truth), fx.w*fx.h, fx.prec, fx.signed)
			compareSamples(t, got, want)

			// Every sample must lie in the component's nominal range.
			lo, hi := int32(0), int32(1)<<fx.prec-1
			if fx.signed {
				lo, hi = -(int32(1) << (fx.prec - 1)), int32(1)<<(fx.prec-1)-1
			}
			for i, v := range got {
				if v < lo || v > hi {
					t.Fatalf("sample[%d]=%d outside [%d,%d]", i, v, lo, hi)
				}
			}
		})
	}
}

// fixtureBytes returns a copy of one fixture codestream.
func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".j2k"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

// findMarker returns the offset of the first occurrence of a marker in
// the main header (before the first SOT).
func findMarker(data []byte, marker uint16) int {
	pos := 2
	for pos+4 <= len(data) {
		m := binary.BigEndian.Uint16(data[pos:])
		if m == marker {
			return pos
		}
		if m == markerSOT {
			return -1
		}
		pos += 2 + int(binary.BigEndian.Uint16(data[pos+2:]))
	}
	return -1
}

// TestDecodeMalformed covers L30: missing EOC, trailing bytes, truncation,
// and header fields out of range. Each must be exact or an error, and
// must never panic.
func TestDecodeMalformed(t *testing.T) {
	base := fixtureBytes(t, "base")
	want := readRaw(t, "testdata/img12u.raw", 45*37, 12, false)

	t.Run("MissingEOC", func(t *testing.T) {
		if !bytes.HasSuffix(base, []byte{0xFF, 0xD9}) {
			t.Fatal("fixture does not end in EOC")
		}
		got, _, _, err := Decode(base[:len(base)-2])
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		compareSamples(t, got, want)
	})
	t.Run("TrailingZeros", func(t *testing.T) {
		d := append(append([]byte(nil), base...), make([]byte, 64)...)
		got, _, _, err := Decode(d)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		compareSamples(t, got, want)
	})
	t.Run("MissingEOCPsotZero", func(t *testing.T) {
		// Psot=0 with no EOC: the tile-part runs to the end of the data.
		d := append([]byte(nil), base[:len(base)-2]...)
		sot := bytes.Index(d, []byte{0xFF, 0x90, 0x00, 0x0A})
		if sot < 0 {
			t.Fatal("no SOT")
		}
		binary.BigEndian.PutUint32(d[sot+6:], 0)
		d[sot+11] = 0 // TNsot unknown
		got, _, _, err := Decode(d)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		compareSamples(t, got, want)
	})
	for _, frac := range []int{10, 50, 90} {
		frac := frac
		t.Run("Truncated"+strconv.Itoa(frac), func(t *testing.T) {
			n := len(base) * frac / 100
			if _, _, _, err := Decode(base[:n]); err == nil {
				t.Fatalf("Decode of %d%% of the stream succeeded, want error", frac)
			}
		})
	}
	t.Run("TooManyResolutions", func(t *testing.T) {
		d := append([]byte(nil), base...)
		cod := findMarker(d, markerCOD)
		d[cod+4+5] = 33 // 34 resolutions
		if _, _, _, err := Decode(d); err == nil {
			t.Fatal("want error for 34 resolutions")
		}
	})
	t.Run("ZeroSize", func(t *testing.T) {
		d := append([]byte(nil), base...)
		siz := findMarker(d, markerSIZ)
		binary.BigEndian.PutUint32(d[siz+4+2:], 0) // Xsiz = 0
		if _, _, _, err := Decode(d); err == nil {
			t.Fatal("want error for Xsiz=0")
		}
	})
	t.Run("ZeroTileSize", func(t *testing.T) {
		d := append([]byte(nil), base...)
		siz := findMarker(d, markerSIZ)
		binary.BigEndian.PutUint32(d[siz+4+18:], 0) // XTsiz = 0
		if _, _, _, err := Decode(d); err == nil {
			t.Fatal("want error for XTsiz=0")
		}
	})
	t.Run("PrecisionTooHigh", func(t *testing.T) {
		d := append([]byte(nil), base...)
		siz := findMarker(d, markerSIZ)
		d[siz+4+36] = 37 // 38 bits
		if _, _, _, err := Decode(d); err == nil {
			t.Fatal("want error for 38-bit precision")
		}
	})
	t.Run("CodeBlockTooLarge", func(t *testing.T) {
		d := append([]byte(nil), base...)
		cod := findMarker(d, markerCOD)
		d[cod+4+6] = 9 // xcb = 11
		if _, _, _, err := Decode(d); err == nil {
			t.Fatal("want error for code-block width exponent 11")
		}
	})
	t.Run("HTCodeBlocks", func(t *testing.T) {
		d := append([]byte(nil), base...)
		cod := findMarker(d, markerCOD)
		d[cod+4+8] = 0x40
		_, _, _, err := Decode(d)
		if err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("err = %v, want unsupported", err)
		}
	})
	t.Run("Empty", func(t *testing.T) {
		for _, d := range [][]byte{nil, {0xFF}, {0xFF, 0x4F}, {0xFF, 0x4F, 0xFF, 0x51}} {
			if _, _, _, err := Decode(d); err == nil {
				t.Fatalf("Decode(%x) succeeded", d)
			}
		}
	})
}

// TestDecodeQCCOverride checks that a QCC marker for component 0 takes
// precedence over the QCD (audit M26). The fixture's QCD is replaced by
// garbage exponents and a QCC with the original exponents is inserted;
// the decode must still be exact.
func TestDecodeQCCOverride(t *testing.T) {
	base := fixtureBytes(t, "base")
	want := readRaw(t, "testdata/img12u.raw", 45*37, 12, false)
	qcd := findMarker(base, markerQCD)
	lqcd := int(binary.BigEndian.Uint16(base[qcd+2:]))
	orig := base[qcd+4 : qcd+2+lqcd] // Sqcd + SPqcd

	// QCC = FF5D Lqcc Cqcc(1 byte) Sqcc SPqcc.
	qcc := []byte{0xFF, 0x5D, 0, 0, 0}
	qcc = append(qcc, orig...)
	binary.BigEndian.PutUint16(qcc[2:], uint16(len(qcc)-2))

	d := append([]byte(nil), base[:qcd+2+lqcd]...)
	for i := qcd + 5; i < qcd+2+lqcd; i++ {
		d[i] = 0x08 // exponent 1 for every band: wrong on purpose
	}
	d = append(d, qcc...)
	d = append(d, base[qcd+2+lqcd:]...)
	got, _, _, err := Decode(d)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	compareSamples(t, got, want)
}
