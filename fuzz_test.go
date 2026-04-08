package grib2

import (
	"bytes"
	"math"
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// CRITICAL / HIGH: FuzzDecodeMessage
// End-to-end GRIB2 message parser. Exercises all section readers and
// dispatches into every Unpack* path depending on the template number
// embedded in the fuzz input.
// ---------------------------------------------------------------------------

func FuzzDecodeMessage(f *testing.F) {
	seed, err := os.ReadFile("testdata/sample.grib2")
	if err == nil {
		f.Add(seed)
	}
	// Add more seeds for broader coverage
	for _, name := range []string{
		"testdata/jpeg.grib2",
		"testdata/ccsds.grib2",
		"testdata/complex.grib2",
		"testdata/png.grib2",
		"testdata/run_length_packing.grib2",
		"testdata/ieee.grib2",
		"testdata/gfs.complex.mvmu.grib2",
	} {
		if data, err := os.ReadFile(name); err == nil {
			f.Add(data)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		dec := NewDecoder(bytes.NewReader(data))
		// Errors are expected on malformed input; panics are bugs.
		msg, err := dec.Decode()
		if err != nil {
			return
		}
		// If parsing succeeded, try to unpack all fields too.
		for i := range msg.Fields {
			msg.Fields[i].Values() //nolint:errcheck
		}
	})
}

// ---------------------------------------------------------------------------
// HIGH: FuzzUnpackJPEG2000
// Fuzz the JPEG2000 unpacker with raw Section 7 data and a realistic
// Template540 extracted from testdata/jpeg.grib2.
// ---------------------------------------------------------------------------

func FuzzUnpackJPEG2000(f *testing.F) {
	raw, err := os.ReadFile("testdata/jpeg.grib2")
	if err != nil {
		f.Skip("testdata/jpeg.grib2 not available")
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		f.Fatalf("ReadMessage: %v", err)
	}
	field := msg.Fields[0]
	f.Add(field.Section7.Data)

	tmpl, ok := field.Section5.Template.(Template540)
	if !ok {
		f.Fatalf("not Template540")
	}
	numValues := field.Section5.NumberOfValues

	f.Fuzz(func(t *testing.T, data []byte) {
		UnpackJPEG2000(data, tmpl, numValues) //nolint:errcheck
	})
}

// ---------------------------------------------------------------------------
// HIGH: FuzzUnpackCCSDS
// Fuzz the CCSDS/AEC unpacker with raw Section 7 data and a realistic
// Template542 extracted from testdata/ccsds.grib2.
// ---------------------------------------------------------------------------

func FuzzUnpackCCSDS(f *testing.F) {
	raw, err := os.ReadFile("testdata/ccsds.grib2")
	if err != nil {
		f.Skip("testdata/ccsds.grib2 not available")
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		f.Fatalf("ReadMessage: %v", err)
	}
	field := msg.Fields[0]
	f.Add(field.Section7.Data)

	tmpl, ok := field.Section5.Template.(Template542)
	if !ok {
		f.Fatalf("not Template542")
	}
	numValues := field.Section5.NumberOfValues

	f.Fuzz(func(t *testing.T, data []byte) {
		UnpackCCSDS(data, tmpl, numValues) //nolint:errcheck
	})
}

// ---------------------------------------------------------------------------
// HIGH: FuzzUnpackComplex
// Fuzz the complex packing unpacker (template 5.3 with spatial differencing)
// using parameters from testdata/gfs.complex.mvmu.grib2.
// ---------------------------------------------------------------------------

func FuzzUnpackComplex(f *testing.F) {
	raw, err := os.ReadFile("testdata/gfs.complex.mvmu.grib2")
	if err != nil {
		f.Skip("testdata/gfs.complex.mvmu.grib2 not available")
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		f.Fatalf("ReadMessage: %v", err)
	}
	field := msg.Fields[0]
	f.Add(field.Section7.Data)

	tmpl53, ok := field.Section5.Template.(Template53)
	if !ok {
		f.Fatalf("not Template53")
	}
	numValues := field.Section5.NumberOfValues

	f.Fuzz(func(t *testing.T, data []byte) {
		UnpackComplex(data, tmpl53.Template52, numValues,
			tmpl53.OrderOfSpatialDifferencing, tmpl53.NumberOfOctetsExtraDescriptors) //nolint:errcheck
	})
}

// ---------------------------------------------------------------------------
// HIGH: FuzzUnpackRunLength
// Fuzz the run-length unpacker using parameters from
// testdata/run_length_packing.grib2.
// ---------------------------------------------------------------------------

func FuzzUnpackRunLength(f *testing.F) {
	raw, err := os.ReadFile("testdata/run_length_packing.grib2")
	if err != nil {
		f.Skip("testdata/run_length_packing.grib2 not available")
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		f.Fatalf("ReadMessage: %v", err)
	}
	field := msg.Fields[0]
	f.Add(field.Section7.Data)

	tmpl, ok := field.Section5.Template.(Template5200)
	if !ok {
		f.Fatalf("not Template5200")
	}
	numValues := field.Section5.NumberOfValues

	f.Fuzz(func(t *testing.T, data []byte) {
		UnpackRunLength(data, tmpl, numValues, math.NaN()) //nolint:errcheck
	})
}

// ---------------------------------------------------------------------------
// MEDIUM: FuzzUnpackSimple
// Fuzz the simple packing unpacker using parameters from
// testdata/sample.grib2.
// ---------------------------------------------------------------------------

func FuzzUnpackSimple(f *testing.F) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		f.Skip("testdata/sample.grib2 not available")
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		f.Fatalf("ReadMessage: %v", err)
	}
	field := msg.Fields[0]
	f.Add(field.Section7.Data)

	tmpl, ok := field.Section5.Template.(Template50)
	if !ok {
		f.Fatalf("not Template50")
	}
	numValues := field.Section5.NumberOfValues

	f.Fuzz(func(t *testing.T, data []byte) {
		UnpackSimple(data, tmpl, numValues) //nolint:errcheck
	})
}

// ---------------------------------------------------------------------------
// MEDIUM: FuzzUnpackPNG
// Fuzz the PNG unpacker with raw PNG bytes. Seeds are generated by
// encoding values from testdata/sample.grib2 via PackPNG.
// ---------------------------------------------------------------------------

func FuzzUnpackPNG(f *testing.F) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		f.Skip("testdata/sample.grib2 not available")
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		f.Fatalf("ReadMessage: %v", err)
	}
	field := msg.Fields[0]
	origTmpl, _ := field.Section5.Template.(Template50)
	origValues, err := UnpackSimple(field.Section7.Data, origTmpl, field.Section5.NumberOfValues)
	if err != nil {
		f.Fatalf("UnpackSimple: %v", err)
	}

	packed, tmpl, err := PackPNG(origValues, 16, 16, 31)
	if err != nil {
		f.Fatalf("PackPNG: %v", err)
	}
	f.Add(packed)

	numValues := uint32(len(origValues))

	f.Fuzz(func(t *testing.T, data []byte) {
		UnpackPNG(data, tmpl, numValues) //nolint:errcheck
	})
}

// TestFuzzSeeds is a simple test that verifies all fuzz seed inputs
// execute without panicking. It can be run with:
//
//	go test -v -count=1 -run TestFuzzSeeds
func TestFuzzSeeds(t *testing.T) {
	t.Run("DecodeMessage", func(t *testing.T) {
		for _, name := range []string{
			"testdata/sample.grib2",
			"testdata/jpeg.grib2",
			"testdata/ccsds.grib2",
			"testdata/complex.grib2",
			"testdata/png.grib2",
			"testdata/run_length_packing.grib2",
			"testdata/ieee.grib2",
			"testdata/gfs.complex.mvmu.grib2",
		} {
			data, err := os.ReadFile(name)
			if err != nil {
				t.Skipf("skip %s: %v", name, err)
			}
			dec := NewDecoder(bytes.NewReader(data))
			msg, err := dec.Decode()
			if err != nil {
				t.Logf("%s: Decode error (expected for some): %v", name, err)
				continue
			}
			for i := range msg.Fields {
				_, err := msg.Fields[i].Values()
				if err != nil {
					t.Logf("%s field %d: Values error: %v", name, i, err)
				}
			}
			t.Logf("%s: OK (%d fields)", name, len(msg.Fields))
		}
	})

	t.Run("UnpackJPEG2000", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/jpeg.grib2")
		if err != nil {
			t.Skipf("skip: %v", err)
		}
		msg, _ := ReadMessage(bytes.NewReader(raw))
		field := msg.Fields[0]
		tmpl := field.Section5.Template.(Template540)
		_, err = UnpackJPEG2000(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
		if err != nil {
			t.Fatalf("UnpackJPEG2000: %v", err)
		}
		t.Log("UnpackJPEG2000 seed: OK")
	})

	t.Run("UnpackCCSDS", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/ccsds.grib2")
		if err != nil {
			t.Skipf("skip: %v", err)
		}
		msg, _ := ReadMessage(bytes.NewReader(raw))
		field := msg.Fields[0]
		tmpl := field.Section5.Template.(Template542)
		_, err = UnpackCCSDS(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
		if err != nil {
			t.Fatalf("UnpackCCSDS: %v", err)
		}
		t.Log("UnpackCCSDS seed: OK")
	})

	t.Run("UnpackComplex", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/gfs.complex.mvmu.grib2")
		if err != nil {
			t.Skipf("skip: %v", err)
		}
		msg, _ := ReadMessage(bytes.NewReader(raw))
		field := msg.Fields[0]
		tmpl := field.Section5.Template.(Template53)
		_, err = UnpackComplex(field.Section7.Data, tmpl.Template52, field.Section5.NumberOfValues,
			tmpl.OrderOfSpatialDifferencing, tmpl.NumberOfOctetsExtraDescriptors)
		if err != nil {
			t.Fatalf("UnpackComplex: %v", err)
		}
		t.Log("UnpackComplex seed: OK")
	})

	t.Run("UnpackRunLength", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/run_length_packing.grib2")
		if err != nil {
			t.Skipf("skip: %v", err)
		}
		msg, _ := ReadMessage(bytes.NewReader(raw))
		field := msg.Fields[0]
		tmpl := field.Section5.Template.(Template5200)
		_, err = UnpackRunLength(field.Section7.Data, tmpl, field.Section5.NumberOfValues, math.NaN())
		if err != nil {
			t.Fatalf("UnpackRunLength: %v", err)
		}
		t.Log("UnpackRunLength seed: OK")
	})

	t.Run("UnpackSimple", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/sample.grib2")
		if err != nil {
			t.Skipf("skip: %v", err)
		}
		msg, _ := ReadMessage(bytes.NewReader(raw))
		field := msg.Fields[0]
		tmpl := field.Section5.Template.(Template50)
		_, err = UnpackSimple(field.Section7.Data, tmpl, field.Section5.NumberOfValues)
		if err != nil {
			t.Fatalf("UnpackSimple: %v", err)
		}
		t.Log("UnpackSimple seed: OK")
	})

	t.Run("UnpackPNG", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/sample.grib2")
		if err != nil {
			t.Skipf("skip: %v", err)
		}
		msg, _ := ReadMessage(bytes.NewReader(raw))
		field := msg.Fields[0]
		origTmpl := field.Section5.Template.(Template50)
		origValues, err := UnpackSimple(field.Section7.Data, origTmpl, field.Section5.NumberOfValues)
		if err != nil {
			t.Fatalf("UnpackSimple: %v", err)
		}
		packed, tmpl, err := PackPNG(origValues, 16, 16, 31)
		if err != nil {
			t.Fatalf("PackPNG: %v", err)
		}
		_, err = UnpackPNG(packed, tmpl, uint32(len(origValues)))
		if err != nil {
			t.Fatalf("UnpackPNG: %v", err)
		}
		t.Log("UnpackPNG seed: OK")
	})
}
