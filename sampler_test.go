package grib2

import (
	"bytes"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// fixtureFields returns every field in testdata whose grid NewSampler
// supports, keyed by "file#msg.field".
func fixtureFields(t testing.TB) map[string]*Field {
	t.Helper()
	paths, _ := filepath.Glob("testdata/*.grib2")
	out := map[string]*Field{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		dec := NewDecoder(bytes.NewReader(raw))
		for m := 0; ; m++ {
			msg, err := dec.Decode()
			if err != nil {
				break
			}
			for i := range msg.Fields {
				f := &msg.Fields[i]
				if _, err := NewSampler(f); err == nil {
					out[filepath.Base(p)+"#"+itoa(m)+"."+itoa(i)] = f
				}
			}
		}
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// Sampler + Interpolate must be bit-identical to BilinearValue, including
// the error cases, at random points inside, near and outside every grid.
func TestSampler_BitIdenticalToBilinearValue(t *testing.T) {
	fields := fixtureFields(t)
	if len(fields) == 0 {
		t.Skip("no supported fixture grids")
	}
	templates := map[string]int{}
	r := rand.New(rand.NewSource(7))
	for name, f := range fields {
		s, err := NewSampler(f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		values, err := f.Values()
		if err != nil {
			continue // packing this decoder does not unpack; nothing to compare
		}
		lats, lons, err := f.GridCoordinates()
		if err != nil || len(lats) == 0 {
			continue
		}
		templates[typeName(f.Section3.Template)]++
		for k := 0; k < 40; k++ {
			// Jitter around real grid points, with some far outside.
			idx := r.Intn(len(lats))
			lat := lats[idx] + (r.Float64()-0.5)*2
			lon := lons[idx] + (r.Float64()-0.5)*2
			if k%8 == 0 {
				lat, lon = r.Float64()*180-90, r.Float64()*360-180
			}
			want, wantErr := f.BilinearValue(lat, lon)
			w, err := s.Weights(lat, lon)
			if err != nil {
				t.Fatalf("%s: Weights(%v,%v): %v", name, lat, lon, err)
			}
			got, gotErr := w.Interpolate(values)
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("%s (%v,%v): error mismatch: BilinearValue %v, sampler %v", name, lat, lon, wantErr, gotErr)
			}
			if wantErr == nil && math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("%s (%v,%v): sampler %v != BilinearValue %v", name, lat, lon, got, want)
			}
		}
	}
	t.Logf("%d fields checked, by template: %v", len(fields), templates)
}

func typeName(v any) string {
	switch v.(type) {
	case Template30:
		return "3.0"
	case Template31:
		return "3.1"
	case Template340:
		return "3.40"
	}
	return "?"
}

func TestSampler_Matches(t *testing.T) {
	fields := fixtureFields(t)
	var a, other *Field
	for _, f := range fields {
		if a == nil {
			a = f
			continue
		}
		if !reflectEqual(a.Section3.Template, f.Section3.Template) {
			other = f
			break
		}
	}
	if a == nil {
		t.Skip("no fixtures")
	}
	s, _ := NewSampler(a)
	if !s.Matches(a) {
		t.Fatal("sampler does not match its own field")
	}
	if other != nil && s.Matches(other) {
		t.Fatal("sampler matches a field on a different grid")
	}
}

func reflectEqual(a, b GridDefinitionTemplate) bool {
	s := &Sampler{grid: a}
	return s.Matches(&Field{Section3: Section3{Template: b}})
}

func TestSampler_OutsideAndNearestCorner(t *testing.T) {
	f := loadSampleField(t)
	s, err := NewSampler(f)
	if err != nil {
		t.Fatal(err)
	}
	lats, lons, _ := f.GridCoordinates()
	mid := len(lats) / 2
	w, _ := s.Weights(lats[mid], lons[mid])
	if w.Outside {
		t.Fatal("interior grid point reported outside")
	}
	if got := w.NearestCorner(); got != mid {
		t.Fatalf("NearestCorner at a grid point = %d, want %d", got, mid)
	}
	w, _ = s.Weights(-89.9, lons[mid])
	if !w.Outside && lats[len(lats)-1] > -89 {
		t.Fatal("point far south of the grid not reported outside")
	}
}

func TestNewSampler_Unsupported(t *testing.T) {
	_, err := NewSampler(&Field{Section3: Section3{Template: Template340{Ni: 0xFFFFFFFF}}})
	if err == nil {
		t.Fatal("reduced Gaussian accepted")
	}
	_, err = NewSampler(&Field{Section3: Section3{Template: Template310{}}})
	if err == nil {
		t.Fatal("Mercator accepted")
	}
}

func BenchmarkBilinearValue(b *testing.B) {
	f := benchField(b)
	for b.Loop() {
		f.BilinearValue(50, -100)
	}
}

func BenchmarkSamplerInterpolate(b *testing.B) {
	f := benchField(b)
	s, _ := NewSampler(f)
	values, _ := f.Values()
	for b.Loop() {
		w, _ := s.Weights(50, -100)
		w.Interpolate(values)
	}
}

func benchField(b *testing.B) *Field {
	for name, f := range fixtureFields(b) {
		if _, ok := f.Section3.Template.(Template31); ok && len(name) > 0 {
			return f
		}
	}
	b.Skip("no rotated fixture")
	return nil
}
