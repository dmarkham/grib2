package grib2

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteMessage_ByteExactAllFixtures decodes every message of every
// testdata/*.grib2 file and requires WriteMessage to reproduce the original
// bytes exactly (eccodes grib_copy round-trips are byte-identical, and so must
// ours be).
func TestWriteMessage_ByteExactAllFixtures(t *testing.T) {
	paths := allFixturePaths(t)
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			dec := NewDecoder(bytes.NewReader(raw))
			off := 0
			for m := 0; ; m++ {
				msg, err := dec.Decode()
				if errors.Is(err, io.EOF) && off == len(raw) {
					break
				}
				if err != nil {
					t.Fatalf("message %d at offset %d: %v", m, off, err)
				}
				n := int(msg.Section0.TotalLength)
				orig := raw[off : off+n]
				var buf bytes.Buffer
				if err := WriteMessage(&buf, msg); err != nil {
					t.Fatalf("message %d: WriteMessage: %v", m, err)
				}
				if got := buf.Bytes(); !bytes.Equal(got, orig) {
					t.Errorf("message %d: rewrite differs: got %d bytes, want %d; first diff at %s",
						m, len(got), len(orig), describeDiff(orig, got))
				}
				off += n
			}
		})
	}
}

// describeDiff reports the first differing offset and which section of orig it falls in.
func describeDiff(orig, got []byte) string {
	i := 0
	for i < len(orig) && i < len(got) && orig[i] == got[i] {
		i++
	}
	sec := "section 0"
	pos := 16
	for pos+5 <= len(orig) && pos <= i {
		if string(orig[pos:pos+4]) == "7777" {
			sec = "section 8"
			break
		}
		l := int(orig[pos])<<24 | int(orig[pos+1])<<16 | int(orig[pos+2])<<8 | int(orig[pos+3])
		if l <= 0 {
			break
		}
		if i < pos+l {
			sec = "section " + itoa(int(orig[pos+4]))
			break
		}
		pos += l
	}
	return "offset " + itoa(i) + " (" + sec + ")"
}

// allFixturePaths returns every GRIB2 fixture under testdata, including the
// upstream-derived ones in testdata/upstream.
func allFixturePaths(t testing.TB) []string {
	t.Helper()
	var paths []string
	for _, pat := range []string{"testdata/*.grib2", "testdata/upstream/*.grib2"} {
		m, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, m...)
	}
	if len(paths) == 0 {
		t.Fatal("no fixtures found")
	}
	return paths
}

// decodeExpectedErrors lists fixtures whose fields Values() must refuse, and
// a fragment of the reason.
var decodeExpectedErrors = map[string]string{
	"testdata/bifourier_complex.grib2":        "unsupported data representation template 53",
	"testdata/spectral_complex.grib2":         "unsupported data representation template 51",
	"testdata/spectral_simple.grib2":          "unsupported data representation template 50",
	"testdata/spherical_model_level.grib2":    "unsupported data representation template 51",
	"testdata/spherical_pressure_level.grib2": "unsupported data representation template 51",
	// Section 7 is empty although bpv=16 and 496 values are declared.
	"testdata/simple_16bit.grib2": "need 992 bytes",
	// Written by eccodes with ccsdsFlags=36 (PAD_RSI) but not actually
	// padded; eccodes cannot decode it either.
	"testdata/ccsds_flags36.grib2": "aec",
	// bitMapIndicator=254 with no earlier bitmap (eccodes: "missing bitmap").
	"testdata/upstream/bitmap_indicator_254.grib2": "254",
	// Grids that do not match their point count.
	"testdata/lambert.grib2":    "349x277",
	"testdata/space_view.grib2": "5424x5424",
	// OpenJPEG writes QCD exponents 32 and 33 into a 5-bit field at these
	// depths (see TestUnpackJPEG2000_OpenJPEGExponentOverflow).
	"testdata/jpeg_bpv30_openjpeg.grib2": "zero bit-plane",
	"testdata/jpeg_bpv31_openjpeg.grib2": "zero bit-plane",
	// Lossy 9/7 JPEG2000 is not supported.
	"testdata/jpeg_lossy_jasper.grib2": "qmfbid",
	// numberOfValues=2 on a 496-point grid.
	"testdata/upstream/numvalues_mismatch.grib2": "declares",
}

// TestValues_AllFixtures decodes every field of every fixture. Each must
// either decode to NumberOfDataPoints values or fail with its listed reason;
// with eccodes available, the values must also match grib_get_data point for
// point (eccodes tests/grib_multi.sh, grib_png.sh, grib_ccsds.sh,
// grib_jpeg.sh, grib_complex.sh compare decoded values the same way).
func TestValues_AllFixtures(t *testing.T) {
	for _, p := range allFixturePaths(t) {
		t.Run(p, func(t *testing.T) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			dec := NewDecoder(bytes.NewReader(raw))
			count := 0
			for m := 0; ; m++ {
				msg, err := dec.Decode()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("message %d: %v", m, err)
				}
				for i := range msg.Fields {
					count++
					f := &msg.Fields[i]
					vals, err := f.Values()
					if want, ok := decodeExpectedErrors[p]; ok {
						if err == nil || !strings.Contains(err.Error(), want) {
							t.Fatalf("message %d field %d: err = %v, want error containing %q", m, i, err, want)
						}
						continue
					}
					if err != nil {
						t.Fatalf("message %d field %d: %v", m, i, err)
					}
					if len(vals) != int(f.Section3.NumberOfDataPoints) {
						t.Fatalf("message %d field %d: %d values, want %d", m, i, len(vals), f.Section3.NumberOfDataPoints)
					}
				}
			}
			if count == 0 {
				t.Fatal("no fields")
			}
		})
	}
}
