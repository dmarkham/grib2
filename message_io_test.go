package grib2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
)

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// countMessages decodes until the first error and returns the count and
// that error.
func countMessages(r io.Reader) (int, error) {
	dec := NewDecoder(r)
	n := 0
	for {
		_, err := dec.Decode()
		if err != nil {
			return n, err
		}
		n++
	}
}

// TestDecode_TruncatedIsNotEOF: a message cut anywhere gives ErrTruncated,
// never io.EOF, and an empty stream gives io.EOF itself (eccodes
// tests/grib_bad.sh, wmo_read_any_from_file.sh: partial message vs clean EOF).
func TestDecode_TruncatedIsNotEOF(t *testing.T) {
	raw := readFixture(t, "testdata/sample.grib2")
	for _, n := range []int{4, 10, 16, 21, 37, 100, len(raw) / 2, len(raw) - 4, len(raw) - 1} {
		_, err := NewDecoder(bytes.NewReader(raw[:n])).Decode()
		if err == nil {
			t.Errorf("cut at %d: no error", n)
			continue
		}
		if errors.Is(err, io.EOF) {
			t.Errorf("cut at %d: error %v matches io.EOF", n, err)
		}
		if !errors.Is(err, ErrTruncated) && !errors.Is(err, ErrInvalidEndSection) {
			t.Errorf("cut at %d: error %v is not ErrTruncated", n, err)
		}
	}
	_, err := NewDecoder(bytes.NewReader(nil)).Decode()
	if err != io.EOF {
		t.Errorf("empty stream: err = %v, want io.EOF", err)
	}
}

// TestDecode_ExactCounts: multi-message files decode to the exact message
// count and then io.EOF (eccodes tests/grib_count.sh, read_any.sh).
func TestDecode_ExactCounts(t *testing.T) {
	for file, want := range map[string]int{
		"testdata/gfs_multifield.grib2":         40,
		"testdata/v.grib2":                      26,
		"testdata/upstream/multi_first3.grib2":  3,
		"testdata/upstream/test_uuid.grib2":     2,
		"testdata/upstream/multi_created.grib2": 1,
	} {
		n, err := countMessages(bytes.NewReader(readFixture(t, file)))
		if err != io.EOF || n != want {
			t.Errorf("%s: %d messages, err %v; want %d and io.EOF", file, n, err, want)
		}
	}
	// v.grib2 is 5.40000 JPEG2000 throughout: every field must decode.
	for i, m := range decodeAll(t, "testdata/v.grib2") {
		if _, err := m.Fields[0].Values(); err != nil {
			t.Fatalf("v.grib2 message %d: %v", i, err)
		}
	}
}

// TestDecode_JunkBetweenMessages: bytes that are not "GRIB" before, between
// and after messages are skipped, and a GRIB edition 1 message is skipped
// with an ErrNotEdition2 error that leaves the stream usable (eccodes
// tests/read_any.sh mixed.grib and pad.grib, wmo_read_any_from_file_with_offset.cc,
// codes_split_file.sh).
func TestDecode_JunkBetweenMessages(t *testing.T) {
	msg := readFixture(t, "testdata/sample.grib2")
	var b bytes.Buffer
	b.WriteString("HEADER junk 12345")
	b.Write(msg)
	b.Write(make([]byte, 7))
	b.WriteString("GRI") // looks like the start of a message but is not
	b.Write(msg)
	b.Write(make([]byte, 3))
	n, err := countMessages(bytes.NewReader(b.Bytes()))
	if n != 2 || err != io.EOF {
		t.Errorf("junk-separated: %d messages, err %v; want 2 and io.EOF", n, err)
	}

	// GRIB1 stub (length 12 in octets 5-7) followed by a GRIB2 message.
	g1 := []byte("GRIB\x00\x00\x0c\x01" + "7777")
	dec := NewDecoder(bytes.NewReader(append(g1, msg...)))
	if _, err := dec.Decode(); !errors.Is(err, ErrNotEdition2) {
		t.Fatalf("GRIB1 message: err = %v, want ErrNotEdition2", err)
	}
	if _, err := dec.Decode(); err != nil {
		t.Fatalf("message after GRIB1: %v", err)
	}
	if _, err := dec.Decode(); err != io.EOF {
		t.Fatalf("end: %v", err)
	}
}

// TestDecode_TotalLengthChecked: a Section 0 total length that does not
// match the message is an error (eccodes tests/grib_bad.sh, grib_repair.sh).
func TestDecode_TotalLengthChecked(t *testing.T) {
	raw := readFixture(t, "testdata/sample.grib2")
	for _, l := range []uint64{100, 999999, 1 << 33, uint64(len(raw)) + 4} {
		bad := append([]byte(nil), raw...)
		binary.BigEndian.PutUint64(bad[8:16], l)
		if _, err := NewDecoder(bytes.NewReader(bad)).Decode(); err == nil {
			t.Errorf("TotalLength %d: no error", l)
		}
	}
	// A wrong length on the first message must not stop the second.
	bad := append([]byte(nil), raw...)
	binary.BigEndian.PutUint64(bad[8:16], uint64(len(raw))+4)
	dec := NewDecoder(bytes.NewReader(append(bad, raw...)))
	if _, err := dec.Decode(); err == nil {
		t.Fatal("first message: no error")
	}
	if _, err := dec.Decode(); err != nil {
		t.Fatalf("second message: %v", err)
	}
}

// TestDecode_CorruptEndSection: a bad 7777 is ErrInvalidEndSection, at the
// end of the stream as well as before another message (eccodes
// tests/grib_bad.sh).
func TestDecode_CorruptEndSection(t *testing.T) {
	if err := ReadSection8(strings.NewReader("XXXX")); !errors.Is(err, ErrInvalidEndSection) {
		t.Errorf("ReadSection8(XXXX) = %v", err)
	}
	if err := ReadSection8(strings.NewReader("77")); err == nil {
		t.Error("ReadSection8 on 2 bytes: no error")
	}
	raw := readFixture(t, "testdata/sample.grib2")
	bad := append([]byte(nil), raw...)
	bad[len(bad)-1] = '6'
	_, err := NewDecoder(bytes.NewReader(bad)).Decode()
	if !errors.Is(err, ErrInvalidEndSection) || errors.Is(err, io.EOF) {
		t.Errorf("at end of stream: err = %v, want ErrInvalidEndSection", err)
	}
	_, err = NewDecoder(bytes.NewReader(append(bad, raw...))).Decode()
	if !errors.Is(err, ErrInvalidEndSection) {
		t.Errorf("before another message: err = %v, want ErrInvalidEndSection", err)
	}
}

// TestDecode_ReaderErrors: errors from the underlying reader come back as
// themselves, and a reader that returns one byte at a time works (eccodes
// tests/wmo_read_any_from_stream.cc, grib_count.sh).
func TestDecode_ReaderErrors(t *testing.T) {
	raw := readFixture(t, "testdata/gfs_multifield.grib2")
	n, err := countMessages(iotest.OneByteReader(bytes.NewReader(raw)))
	if n != 40 || err != io.EOF {
		t.Errorf("one byte at a time: %d messages, err %v", n, err)
	}
	errBoom := errors.New("boom")
	one := readFixture(t, "testdata/sample.grib2") // 1179 bytes, longer than 500
	_, err = NewDecoder(io.MultiReader(bytes.NewReader(one[:500]), iotest.ErrReader(errBoom))).Decode()
	if !errors.Is(err, errBoom) || errors.Is(err, io.EOF) {
		t.Errorf("reader error: err = %v, want errBoom", err)
	}
	dir, err := os.Open("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if _, err := NewDecoder(dir).Decode(); err == nil || errors.Is(err, io.EOF) {
		t.Errorf("reading a directory: err = %v", err)
	}
}

// TestReadSection_SizeLimits: implausible section lengths are refused
// without allocating them (eccodes tests/grib_set_large_message_fail.cc).
func TestReadSection_SizeLimits(t *testing.T) {
	hdr := func(length uint32, num byte) []byte {
		b := make([]byte, 5)
		binary.BigEndian.PutUint32(b, length)
		b[4] = num
		return b
	}
	if _, err := ReadSection7(bytes.NewReader(hdr(256<<20+6, 7))); err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("256 MiB section: err = %v", err)
	}
	for _, l := range []uint32{0, 1, 4} {
		if _, err := ReadSection4(bytes.NewReader(append(hdr(l, 4), make([]byte, 40)...))); err == nil {
			t.Errorf("section length %d: no error", l)
		}
	}
}

// TestWriteMessage_AfterModification: changing a field rewrites only what
// changed and keeps the lengths consistent (eccodes tests/grib_copy_message.cc,
// grib_update_sections_lengths.sh).
func TestWriteMessage_AfterModification(t *testing.T) {
	raw := readFixture(t, "testdata/sample.grib2")
	msg := decodeAll(t, "testdata/sample.grib2")[0]
	msg.Section1.Hour = 18
	var b bytes.Buffer
	if err := WriteMessage(&b, msg); err != nil {
		t.Fatal(err)
	}
	diff := 0
	for i := range raw {
		if raw[i] != b.Bytes()[i] {
			diff++
		}
	}
	if b.Len() != len(raw) || diff != 1 {
		t.Errorf("Hour change: %d bytes differ, length %d vs %d", diff, b.Len(), len(raw))
	}

	msg = decodeAll(t, "testdata/sample.grib2")[0]
	msg.Fields[0].Section7.Data = msg.Fields[0].Section7.Data[:10]
	b.Reset()
	if err := WriteMessage(&b, msg); err != nil {
		t.Fatal(err)
	}
	out := b.Bytes()
	if binary.BigEndian.Uint64(out[8:16]) != uint64(len(out)) {
		t.Errorf("TotalLength %d, message %d bytes", binary.BigEndian.Uint64(out[8:16]), len(out))
	}
	s7 := out[len(out)-4-15 : len(out)-4]
	if binary.BigEndian.Uint32(s7[:4]) != 15 || s7[4] != 7 {
		t.Errorf("section 7 header % x, want length 15", s7[:5])
	}
	if _, err := NewDecoder(bytes.NewReader(out)).Decode(); err != nil {
		t.Errorf("re-decode: %v", err)
	}
}

// TestValues_Concurrent decodes shared fields from many goroutines; run
// with -race (eccodes tests/grib_encode_threads.cc, grib_threads_ecc-604-encode.cc).
func TestValues_Concurrent(t *testing.T) {
	files := []string{"testdata/jpeg.grib2", "testdata/ccsds.grib2", "testdata/upstream/png_bpv16.grib2",
		"testdata/complex.grib2", "testdata/gfs.complex.mvmu.grib2", "testdata/sample.grib2"}
	var fields []*Field
	var want [][]float64
	for _, f := range files {
		fld := &decodeAll(t, f)[0].Fields[0]
		v, err := fld.Values()
		if err != nil {
			t.Fatal(err)
		}
		fields = append(fields, fld)
		want = append(want, v)
	}
	packWant, _, err := PackSimple(want[5], 16)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := 0; it < 3; it++ {
				for k, f := range fields {
					v, err := f.Values()
					if err != nil {
						errs <- err
						return
					}
					for i := range v {
						if v[i] != want[k][i] && !(v[i] != v[i] && want[k][i] != want[k][i]) {
							errs <- errors.New(files[k] + ": values differ between goroutines")
							return
						}
					}
				}
				p, _, err := PackSimple(want[5], 16)
				if err != nil || !bytes.Equal(p, packWant) {
					errs <- errors.New("PackSimple differs between goroutines")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
