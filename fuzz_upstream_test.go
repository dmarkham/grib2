package grib2

import (
	"bytes"
	"math"
	"os"
	"testing"
)

// FuzzDecodeStream decodes every message of the input, and for each one
// checks the invariants upstream's tools rely on (eccodes tests/grib_count.sh,
// libaec fuzzing/fuzz_target.cc, openjpeg tests/fuzzers): no panic, a
// decoded message rewrites to bytes that decode to the same rewrite, and
// Values/GridCoordinates/lookups never panic.
func FuzzDecodeStream(f *testing.F) {
	for _, name := range []string{
		"testdata/sample.grib2",
		"testdata/gfs_multifield.grib2",
		"testdata/upstream/multi_created.grib2",
		"testdata/upstream/test_uuid.grib2",
		"testdata/upstream/section1_calendar.grib2",
		"testdata/upstream/bitmap_indicator_254.grib2",
		"testdata/upstream/pdt8_3ranges.grib2",
		"testdata/upstream/ll_scan32.grib2",
		"testdata/complex.grib2",
		"testdata/png.grib2",
		"testdata/ieee.grib2",
		"testdata/v.grib2",
		"testdata/upstream/multi_first3.grib2",
	} {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		f.Add(data)
		f.Add(append([]byte("junk"), data...))
		f.Add(data[:len(data)/2])
		f.Add(append(append([]byte(nil), data...), 0, 0, 0))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		dec := NewDecoder(bytes.NewReader(data))
		for n := 0; n < 16; n++ {
			msg, err := dec.Decode()
			if err != nil {
				return
			}
			var b1 bytes.Buffer
			if err := WriteMessage(&b1, msg); err != nil {
				continue
			}
			msg2, err := ReadMessage(bytes.NewReader(b1.Bytes()))
			if err != nil {
				t.Fatalf("rewrite of a decoded message does not decode: %v", err)
			}
			var b2 bytes.Buffer
			if err := WriteMessage(&b2, msg2); err != nil || !bytes.Equal(b1.Bytes(), b2.Bytes()) {
				t.Fatalf("rewrite is not stable: %v", err)
			}
			for i := range msg.Fields {
				fld := &msg.Fields[i]
				if fld.Section3.NumberOfDataPoints > 1<<16 {
					continue // keep iterations fast
				}
				fld.Values()              //nolint:errcheck
				fld.GridCoordinates()     //nolint:errcheck
				fld.NearestValue(10, 20)  //nolint:errcheck
				fld.BilinearValue(10, 20) //nolint:errcheck
			}
		}
	})
}

// FuzzUnpackComplexTemplate fuzzes the Template 5.2/5.3 parameters as well
// as the data (the audit's H22: octets=0 used to panic).
func FuzzUnpackComplexTemplate(f *testing.F) {
	raw, err := os.ReadFile("testdata/gfs.complex.mvmu.grib2")
	if err != nil {
		f.Skip("fixture missing")
	}
	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		f.Fatal(err)
	}
	t53 := msg.Fields[0].Section5.Template.(Template53)
	data := msg.Fields[0].Section7.Data[:4096]
	f.Add(data, uint8(2), uint8(2), t53.BitsPerValue, t53.ReferenceForGroupWidths, t53.NumberOfBitsForGroupWidths,
		t53.NumberOfBitsForScaledGroupLengths, t53.MissingValueManagement, uint32(200), uint32(1000))
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8}, uint8(1), uint8(0), uint8(8), uint8(0), uint8(4), uint8(3), uint8(1), uint32(3), uint32(10))
	f.Fuzz(func(t *testing.T, data []byte, order, octets, bpv, refW, nbW, nbL, mvm uint8, groups, n uint32) {
		if n > 1<<16 || groups > 1<<16 {
			return
		}
		tm := t53.Template52
		tm.BitsPerValue, tm.ReferenceForGroupWidths, tm.NumberOfBitsForGroupWidths = bpv, refW, nbW
		tm.NumberOfBitsForScaledGroupLengths, tm.MissingValueManagement, tm.NumberOfGroups = nbL, mvm, groups
		tm.TrueLengthOfLastGroup = n / (groups + 1)
		vals, err := UnpackComplex(data, tm, n, order, octets)
		if err == nil && len(vals) != int(n) {
			t.Fatalf("%d values, want %d", len(vals), n)
		}
	})
}

// FuzzUnpackRunLengthTemplate fuzzes the Template 5.200 parameters (the
// audit's M22: bpv 0 divided by zero).
func FuzzUnpackRunLengthTemplate(f *testing.F) {
	f.Add([]byte{1, 2, 3, 250, 2}, uint8(8), uint16(3), uint16(3), uint8(0), uint16(5))
	f.Add([]byte{1, 2, 3}, uint8(0), uint16(3), uint16(3), uint8(1), uint16(3))
	f.Fuzz(func(t *testing.T, data []byte, bpv uint8, maxLevel, numLevels uint16, dsf uint8, n uint16) {
		levels := make([]uint16, int(numLevels)%64)
		for i := range levels {
			levels[i] = uint16(i * 10)
		}
		tm := Template5200{BitsPerValue: bpv, MaxLevelValue: maxLevel, NumberOfLevelValues: numLevels,
			DecimalScaleFactor: dsf, LevelValues: levels}
		vals, err := UnpackRunLength(data, tm, uint32(n), math.NaN())
		if err == nil && len(vals) != int(n) {
			t.Fatalf("%d values, want %d", len(vals), n)
		}
	})
}
