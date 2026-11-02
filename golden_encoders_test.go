package grib2

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// goldenDir holds messages written by our encoders. They were decoded by
// eccodes (with Jasper and with OpenJPEG for JPEG2000) when the reference
// was generated, and eccodes' results are in eccodesReferenceFile like any
// other fixture. TestEncoders_GoldenBytes checks the encoders still write
// exactly these bytes, so the eccodes check holds without eccodes installed
// (eccodes tests/grib_packing_order.sh, grib_jpeg.sh, grib_ccsds.sh).
const goldenDir = "testdata/upstream/encoded"

type goldenVariant struct {
	name  string
	build func(t *testing.T) []byte
}

// goldenMessage replaces the data of the first field of base with pack's
// output and returns the encoded message.
func goldenMessage(t *testing.T, base string, dropBitmap bool, pack func(f *Field, vals []float64) error) []byte {
	t.Helper()
	msg := decodeAll(t, base)[0]
	f := msg.Fields[0]
	full, err := f.Values()
	if err != nil {
		t.Fatal(err)
	}
	vals := full
	if dropBitmap && f.Section6.HasBitmap() {
		// Complex packing: carry the missing points as NaN instead.
		f.Section6 = Section6{Indicator: 255}
	} else {
		vals = nil
		for _, v := range full {
			if !math.IsNaN(v) {
				vals = append(vals, v)
			}
		}
	}
	if err := pack(&f, vals); err != nil {
		t.Fatal(err)
	}
	msg.Fields = []Field{f}
	var b bytes.Buffer
	if err := WriteMessage(&b, msg); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func goldenVariants() []goldenVariant {
	const rll = "testdata/regular_latlon_surface.grib2"
	const gcsd = "testdata/grid_complex_spatial_differencing.grib2"
	set5 := func(f *Field, number uint16, n int, tmpl DataRepresentationTemplate, data []byte) {
		f.Section5 = Section5{TemplateNumber: number, NumberOfValues: uint32(n), Template: tmpl}
		f.Section7 = Section7{Data: data}
	}
	var out []goldenVariant
	add := func(name, base string, dropBitmap bool, pack func(f *Field, v []float64) error) {
		out = append(out, goldenVariant{name, func(t *testing.T) []byte { return goldenMessage(t, base, dropBitmap, pack) }})
	}
	for _, bpv := range []uint8{7, 16, 24} {
		bpv := bpv
		add("simple_b"+itoa(int(bpv)), rll, false, func(f *Field, v []float64) error {
			d, tm, err := PackSimple(v, bpv)
			set5(f, 0, len(v), tm, d)
			return err
		})
	}
	for _, base := range []struct{ name, file string }{{"rll", rll}, {"nan", gcsd}} {
		base := base
		add("complex_"+base.name, base.file, true, func(f *Field, v []float64) error {
			d, tm, err := PackComplex(v, 16)
			set5(f, 2, len(v), tm, d)
			return err
		})
		for _, order := range []uint8{1, 2} {
			order := order
			add("spatial"+itoa(int(order))+"_"+base.name, base.file, true, func(f *Field, v []float64) error {
				d, tm, err := PackComplexSpatialDiff(v, 16, order)
				set5(f, 3, len(v), tm, d)
				return err
			})
		}
	}
	for _, prec := range []uint8{1, 2} {
		prec := prec
		add("ieee"+itoa(32*int(prec)), rll, false, func(f *Field, v []float64) error {
			d, tm, err := PackIEEE(v, prec)
			set5(f, 4, len(v), tm, d)
			return err
		})
	}
	add("ccsds_b16", rll, false, func(f *Field, v []float64) error {
		d, tm, err := PackCCSDS(v, 16, 32, 128)
		set5(f, 42, len(v), tm, d)
		return err
	})
	add("ccsds_b24_bitmap", gcsd, false, func(f *Field, v []float64) error {
		d, tm, err := PackCCSDS(v, 24, 32, 128)
		set5(f, 42, len(v), tm, d)
		return err
	})
	add("png_b16", rll, false, func(f *Field, v []float64) error {
		t30 := f.Section3.Template.(Template30)
		d, tm, err := PackPNG(v, 16, int(t30.Ni), int(t30.Nj))
		set5(f, 41, len(v), tm, d)
		return err
	})
	// JPEG2000: the cases of the former TestPackJPEG2000_EccodesDecodes.
	// "shifted" adds 0.1 so the minimum is not a float32.
	for _, c := range []struct {
		name  string
		shift float64
		bpv   uint8
		cons  bool
	}{{"jpeg_b8", 0, 8, false}, {"jpeg_b14", 0, 14, false}, {"jpeg_shifted_b16", 0.1, 16, false},
		{"jpeg_shifted_b24", 0.1, 24, false}, {"jpeg_shifted_b28", 0.1, 28, false},
		{"jpeg_shifted_b29", 0.1, 29, false}, {"jpeg_shifted_b30", 0.1, 30, false}, {"jpeg_constant", 0, 12, true}} {
		c := c
		add(c.name, rll, false, func(f *Field, v []float64) error {
			vals := make([]float64, len(v))
			for i := range v {
				vals[i] = v[i] + c.shift
				if c.cons {
					vals[i] = 273.15
				}
			}
			d, tm, err := PackJPEG2000(vals, c.bpv)
			set5(f, 40, len(vals), tm, d)
			return err
		})
	}
	return out
}

// writeGoldenEncoded writes every golden variant into goldenDir.
func writeGoldenEncoded(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, v := range goldenVariants() {
		if err := os.WriteFile(filepath.Join(goldenDir, v.name+".grib2"), v.build(t), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestEncoders_GoldenBytes: every encoder still writes the bytes eccodes
// was checked against. If an encoder change is intended, regenerate with
// GRIB2_WRITE_ECCODES_REFERENCE=1 (needs eccodes) so the new output is
// decoded by eccodes again.
func TestEncoders_GoldenBytes(t *testing.T) {
	variants := goldenVariants()
	files, _ := filepath.Glob(filepath.Join(goldenDir, "*.grib2"))
	if len(files) != len(variants) {
		t.Fatalf("%d golden files for %d variants", len(files), len(variants))
	}
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(goldenDir, v.name+".grib2"))
			if err != nil {
				t.Fatal(err)
			}
			if got := v.build(t); !bytes.Equal(got, want) {
				t.Errorf("encoder output changed: %s", describeDiff(want, got))
			}
		})
	}
}
