package grib2

import (
	"fmt"
	"math"
)

// WindRotation turns the u and v components of a vector field (wind, wind
// stress, currents) from the grid's own axes to true east and north.
//
// Many models on projected or rotated grids store u and v relative to the
// grid: u along the grid's x (or i) axis and v along its y (or j) axis.
// HRRR and NAM (Lambert conformal) and the Canadian RDPS and HRDPS (rotated
// lat/lon) do. Flag Table 3.3 bit 5 in the grid definition says which it
// is. Speed is the same either way, but a direction worked out from
// grid-relative u and v is wrong by the angle between grid north and true
// north, which changes from place to place: across HRRR it runs from 22.8
// degrees west of true north to 22.8 degrees east.
//
// A WindRotation is built from the grid definition of either component
// field. It is immutable and safe for concurrent use.
//
// The grid's axes are those of its projection: x towards the grid's east and
// y towards the grid's north, whatever order the scanning mode stores the
// points in.
type WindRotation struct {
	gridRelative bool
	north        func(lat, lon float64) float64 // radians clockwise from true north
}

// NewWindRotation builds the rotation for the grid f is defined on:
// regular and rotated lat/lon, regular and reduced Gaussian, Mercator, polar
// stereographic and Lambert conformal grids.
func NewWindRotation(f *Field) (*WindRotation, error) {
	const deg2rad = math.Pi / 180
	zero := func(_, _ float64) float64 { return 0 }
	switch t := f.Section3.Template.(type) {
	case Template30:
		return &WindRotation{gridRelative: gridRelativeFlag(t.ResolutionAndComponentFlags), north: zero}, nil
	case Template340:
		return &WindRotation{gridRelative: gridRelativeFlag(t.ResolutionAndComponentFlags), north: zero}, nil
	case Template31:
		// Grid north is towards the north pole of the rotated system: the
		// point opposite the southern pole of the grid definition, moved
		// west by the angle of rotation, which GridCoordinates (like
		// eccodes) takes off every longitude after unrotating.
		spLat, spLon, angle := rotation(t)
		poleLat, poleLon := -spLat*deg2rad, (spLon+180-angle)*deg2rad
		sinPole, cosPole := math.Sin(poleLat), math.Cos(poleLat)
		return &WindRotation{gridRelative: gridRelativeFlag(t.ResolutionAndComponentFlags),
			north: func(lat, lon float64) float64 {
				phi, dl := lat*deg2rad, poleLon-lon*deg2rad
				// Initial bearing of the great circle from the point to the pole.
				return math.Atan2(math.Sin(dl)*cosPole, math.Cos(phi)*sinPole-math.Sin(phi)*cosPole*math.Cos(dl))
			}}, nil
	case Template310, Template320, Template330:
		g, _, err := projectedGridOf(t)
		if err != nil {
			return nil, fmt.Errorf("grib2: NewWindRotation: %w", err)
		}
		return &WindRotation{gridRelative: gridRelativeFlag(componentFlags(t)), north: g.north}, nil
	default:
		return nil, fmt.Errorf("grib2: NewWindRotation: not supported for template %T", f.Section3.Template)
	}
}

// componentFlags returns the resolution and component flags of a projected
// grid's definition.
func componentFlags(tmpl GridDefinitionTemplate) uint8 {
	switch t := tmpl.(type) {
	case Template310:
		return t.ResolutionAndComponentFlags
	case Template320:
		return t.ResolutionAndComponentFlags
	case Template330:
		return t.ResolutionAndComponentFlags
	}
	return 0
}

// gridRelativeFlag reads bit 5 of Flag Table 3.3: the u and v components of
// vector quantities are resolved relative to the grid (1) or to easterly
// and northerly directions (0).
func gridRelativeFlag(resolutionAndComponentFlags uint8) bool {
	return resolutionAndComponentFlags&0x08 != 0
}

// GridRelative reports whether the grid definition says its vector
// components are relative to the grid (Flag Table 3.3 bit 5). When it is
// false the components are already east and north, and ToEarth returns them
// unchanged.
func (r *WindRotation) GridRelative() bool { return r.gridRelative }

// GridNorth returns the direction of the grid's y axis at (lat, lon), in
// degrees clockwise from true north, between -180 and 180: 12 means grid
// north points 12 degrees east of true north there, -12 that it points 12
// degrees west. It describes the grid's geometry and does not depend on
// GridRelative. The longitude may be given in any range.
func (r *WindRotation) GridNorth(lat, lon float64) float64 {
	return r.north(lat, lon) * 180 / math.Pi
}

// ToEarth converts the components u and v that the fields hold at
// (lat, lon) to the eastward and northward components there. Values sampled
// from the u and v fields by any method (nearest point, bilinear) are
// converted with the angle at the place they were sampled for.
//
// If the grid's components are already east and north (GridRelative is
// false), ToEarth returns u and v unchanged.
func (r *WindRotation) ToEarth(u, v, lat, lon float64) (east, north float64) {
	if !r.gridRelative {
		return u, v
	}
	sin, cos := math.Sincos(r.north(lat, lon))
	return u*cos + v*sin, v*cos - u*sin
}
