package grib2

import (
	"fmt"
	"math"
	"reflect"
)

// BilinearWeights is the result of locating a point on a grid for bilinear
// interpolation: the four surrounding grid indices and the point's
// fractional position inside that cell. It holds no data values, so one
// BilinearWeights can be applied to every field that shares the grid.
//
// Index order is v00, v10, v01, v11: (i0,j0), (i0+1,j0), (i0,j0+1),
// (i0+1,j0+1), as indices into the canonical slice returned by
// Field.Values.
type BilinearWeights struct {
	Index        [4]int
	XFrac, YFrac float64
	// Outside is true when the point lies beyond the grid edge. The
	// indices and fractions are then clamped to the nearest edge cell,
	// which is what BilinearValue has always returned for such points.
	Outside bool
}

// cornerWeights turns fractional grid coordinates into BilinearWeights,
// clamping to the grid exactly as the bilinear lookups always have.
func cornerWeights(fi, fj float64, ni, nj int) BilinearWeights {
	outside := fi < 0 || fj < 0 || fi > float64(ni-1) || fj > float64(nj-1)

	i0 := int(math.Floor(fi))
	j0 := int(math.Floor(fj))
	xFrac := fi - float64(i0)
	yFrac := fj - float64(j0)

	if i0 < 0 {
		i0 = 0
		xFrac = 0
	}
	if i0 >= ni-1 {
		i0 = ni - 2
		xFrac = 1
	}
	if j0 < 0 {
		j0 = 0
		yFrac = 0
	}
	if j0 >= nj-1 {
		j0 = nj - 2
		yFrac = 1
	}

	i1 := i0 + 1
	j1 := j0 + 1
	return BilinearWeights{
		Index:   [4]int{j0*ni + i0, j0*ni + i1, j1*ni + i0, j1*ni + i1},
		XFrac:   xFrac,
		YFrac:   yFrac,
		Outside: outside,
	}
}

// Interpolate applies the weights to a field's values (as returned by
// Field.Values). It returns an error if any of the four corners is
// missing (NaN) or out of range, exactly like BilinearValue.
func (w BilinearWeights) Interpolate(values []float64) (float64, error) {
	v00 := gridVal(values, w.Index[0])
	v10 := gridVal(values, w.Index[1])
	v01 := gridVal(values, w.Index[2])
	v11 := gridVal(values, w.Index[3])

	if math.IsNaN(v00) || math.IsNaN(v10) || math.IsNaN(v01) || math.IsNaN(v11) {
		return 0, fmt.Errorf("grib2: BilinearValue: one or more surrounding grid points are missing (NaN)")
	}

	xFrac, yFrac := w.XFrac, w.YFrac
	return v00*(1-xFrac)*(1-yFrac) +
		v10*xFrac*(1-yFrac) +
		v01*(1-xFrac)*yFrac +
		v11*xFrac*yFrac, nil
}

// NearestCorner returns the index of the corner with the largest bilinear
// weight. It is the right lookup for categorical fields (precipitation
// type, land mask) where averaging codes is meaningless.
func (w BilinearWeights) NearestCorner() int {
	wts := [4]float64{
		(1 - w.XFrac) * (1 - w.YFrac),
		w.XFrac * (1 - w.YFrac),
		(1 - w.XFrac) * w.YFrac,
		w.XFrac * w.YFrac,
	}
	best := 0
	for k := 1; k < 4; k++ {
		if wts[k] > wts[best] {
			best = k
		}
	}
	return w.Index[best]
}

// Sampler locates points on one grid without touching data values.
//
// Field.BilinearValue unpacks the whole field on every call, which is fine
// for one point but far too slow for rendering or for many fields on the
// same grid. A Sampler is built once from a field's grid definition; its
// Weights can then be computed for any number of points and applied to the
// Values of any field on the same grid. Sampler.Weights(lat, lon) followed
// by Interpolate(f.Values()) returns bit-identical results to
// f.BilinearValue(lat, lon).
//
// Supported grids are those BilinearValue supports: regular lat/lon (3.0),
// rotated lat/lon (3.1) and regular Gaussian (3.40 with Ni set). A Sampler
// is immutable and safe for concurrent use.
type Sampler struct {
	grid    any // Section3.Template the sampler was built from
	weights func(lat, lon float64) (BilinearWeights, error)
}

// NewSampler builds a Sampler for the grid that f is defined on.
func NewSampler(f *Field) (*Sampler, error) {
	switch tmpl := f.Section3.Template.(type) {
	case Template30:
		return &Sampler{grid: tmpl, weights: func(lat, lon float64) (BilinearWeights, error) {
			return bilinearWeightsLatLon(tmpl, lat, lon)
		}}, nil
	case Template31:
		return &Sampler{grid: tmpl, weights: func(lat, lon float64) (BilinearWeights, error) {
			return bilinearWeightsRotatedLatLon(tmpl, lat, lon)
		}}, nil
	case Template340:
		if tmpl.Ni == 0xFFFFFFFF {
			return nil, fmt.Errorf("grib2: NewSampler: reduced Gaussian grids are not supported")
		}
		return &Sampler{grid: tmpl, weights: func(lat, lon float64) (BilinearWeights, error) {
			return bilinearWeightsRegularGaussian(tmpl, lat, lon)
		}}, nil
	default:
		return nil, fmt.Errorf("grib2: NewSampler: bilinear interpolation not supported for template %T", tmpl)
	}
}

// Weights locates (lat, lon) in degrees on the sampler's grid.
func (s *Sampler) Weights(lat, lon float64) (BilinearWeights, error) {
	return s.weights(lat, lon)
}

// Matches reports whether f is defined on exactly the same grid as the
// sampler, so the sampler's weights are valid for f.Values().
func (s *Sampler) Matches(f *Field) bool {
	return reflect.DeepEqual(s.grid, f.Section3.Template)
}
