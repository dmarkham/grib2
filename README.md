# grib2

A pure Go library for reading and writing GRIB2 (WMO FM 92 GRIB Edition 2) files.

[![Go Reference](https://pkg.go.dev/badge/github.com/dmarkham/grib2.svg)](https://pkg.go.dev/github.com/dmarkham/grib2)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Overview

GRIB2 is the World Meteorological Organization's binary format for gridded meteorological data, used by weather services worldwide (NOAA, ECMWF, JMA, DWD, etc.). This library provides a complete, idiomatic Go implementation for both decoding and encoding GRIB2 messages -- no CGo, no external C libraries, no system dependencies.

## Features

### Packing Formats

| Format | Template | Decode | Encode |
|--------|----------|--------|--------|
| Simple packing | 5.0 | Yes | Yes |
| Complex packing | 5.2 | Yes | Yes |
| Complex packing + spatial differencing | 5.3 | Yes | Yes |
| IEEE floating point (32/64-bit) | 5.4 | Yes | Yes |
| JPEG2000 | 5.40 | Yes | Yes |
| PNG | 5.41 | Yes | Yes |
| CCSDS/AEC (Adaptive Entropy Coding) | 5.42 | Yes | Yes |
| Run-length packing with level values | 5.200 | Yes | -- |

### Grid Definition Templates (Section 3)

| Grid Type | Template |
|-----------|----------|
| Latitude/Longitude (equidistant cylindrical) | 3.0 |
| Rotated Latitude/Longitude | 3.1 |
| Mercator | 3.10 |
| Polar Stereographic | 3.20 |
| Lambert Conformal | 3.30 |
| Gaussian Latitude/Longitude (regular and reduced) | 3.40 |
| Space View Perspective (geostationary satellite) | 3.90 |

### Product Definition Templates (Section 4)

| Product Type | Template |
|--------------|----------|
| Analysis or forecast at a point in time | 4.0 |
| Individual ensemble forecast | 4.1 |
| Derived ensemble forecast | 4.2 |
| Statistically processed values over time interval | 4.8 |
| Ensemble forecast over time interval | 4.11 |
| Derived ensemble over time interval | 4.12 |

### High-Level API

- **Bitmap support** -- automatic expansion of bitmap-masked fields (NaN for missing points)
- **Scanning mode reordering** -- all 16 scanning mode combinations normalized to canonical west-to-east, north-to-south order
- **Grid coordinates** -- `GridCoordinates()` returns lat/lon arrays for all supported grid types, including projected grids (Lambert, Polar Stereographic, Mercator, Space View)
- **Nearest-neighbour lookup** -- `NearestValue(lat, lon)` with O(1) direct index computation for regular grids, haversine fallback for others
- **Bilinear interpolation** -- `BilinearValue(lat, lon)` for smooth value estimation between grid points
- **Code table lookups** -- human-readable names for disciplines, centres, parameters, surface types, and all template numbers
- **Multi-message files** -- `Decoder` reads files containing any number of concatenated GRIB2 messages
- **Byte-perfect round-trips** -- `ReadMessage` followed by `WriteMessage` produces identical bytes

## Installation

```
go get github.com/dmarkham/grib2
```

Requires Go 1.25 or later. Zero external dependencies outside the standard library.

## Quick Start

### Read a GRIB2 file and extract values

```go
package main

import (
	"fmt"
	"os"

	"github.com/dmarkham/grib2"
)

func main() {
	f, err := os.Open("forecast.grib2")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	dec := grib2.NewDecoder(f)
	for {
		msg, err := dec.Decode()
		if err != nil {
			break // io.EOF or parse error
		}

		fmt.Printf("Discipline: %s\n", grib2.DisciplineName(msg.Section0.Discipline))
		fmt.Printf("Centre:     %s\n", grib2.CentreName(msg.Section1.Centre))
		fmt.Printf("Reference:  %s\n", msg.Section1.ReferenceTime())

		for i, field := range msg.Fields {
			values, err := field.Values()
			if err != nil {
				fmt.Printf("  field %d: unpack error: %v\n", i, err)
				continue
			}
			fmt.Printf("  field %d: %d values, grid template %d, packing template %d\n",
				i, len(values), field.Section3.TemplateNumber, field.Section5.TemplateNumber)
		}
	}
}
```

### Query a specific location (nearest-neighbour)

```go
field := msg.Fields[0]

// Find the grid point closest to Denver, CO
value, idx, lat, lon, err := field.NearestValue(39.7392, -104.9903)
if err != nil {
	panic(err)
}
fmt.Printf("Nearest grid point: (%.4f, %.4f) index=%d value=%.2f\n", lat, lon, idx, value)

// Or use bilinear interpolation for smoother results
interpolated, err := field.BilinearValue(39.7392, -104.9903)
if err != nil {
	panic(err)
}
fmt.Printf("Interpolated value: %.2f\n", interpolated)
```

### Write a GRIB2 file

```go
package main

import (
	"os"

	"github.com/dmarkham/grib2"
)

func main() {
	// Create a simple 4x3 temperature grid
	values := []float64{
		280.0, 281.0, 282.0, 283.0,
		279.0, 280.5, 281.5, 282.5,
		278.0, 279.5, 280.5, 281.0,
	}

	// Pack with simple packing at 16 bits per value
	packed, tmpl50, err := grib2.PackSimple(values, 16)
	if err != nil {
		panic(err)
	}

	msg := &grib2.Message{
		Section0: grib2.Section0{
			Discipline: 0, // Meteorological
			Edition:    2,
		},
		Section1: grib2.Section1{
			Centre:              7,  // NCEP
			MasterTablesVersion: 2,
			Year:                2025, Month: 1, Day: 15,
			Hour: 12, Minute: 0, Second: 0,
		},
		Fields: []grib2.Field{{
			Section3: grib2.Section3{
				NumberOfDataPoints: 12,
				TemplateNumber:     0,
				Template: grib2.Template30{
					Ni: 4, Nj: 3,
					LatitudeOfFirstGridPoint:  40000000,  // 40.0N
					LongitudeOfFirstGridPoint: 350000000, // 350.0E (-10.0)
					LatitudeOfLastGridPoint:   38000000,  // 38.0N
					LongitudeOfLastGridPoint:  353000000, // 353.0E (-7.0)
					IDirectionIncrement:       1000000,   // 1.0 degree
					JDirectionIncrement:       1000000,   // 1.0 degree
				},
			},
			Section4: grib2.Section4{
				TemplateNumber: 0,
				Template: grib2.Template40{
					ParameterCategory: 0, // Temperature
					ParameterNumber:   0, // Temperature (K)
				},
			},
			Section5: grib2.Section5{
				NumberOfValues:  12,
				TemplateNumber: 0,
				Template:       tmpl50,
			},
			Section6: grib2.Section6{Indicator: 255}, // no bitmap
			Section7: grib2.Section7{Data: packed},
		}},
	}

	out, err := os.Create("output.grib2")
	if err != nil {
		panic(err)
	}
	defer out.Close()

	if err := grib2.WriteMessage(out, msg); err != nil {
		panic(err)
	}
}
```

## API Reference

### Reading

| Function/Type | Description |
|---------------|-------------|
| `NewDecoder(r io.Reader) *Decoder` | Create a decoder for reading multiple messages from a stream |
| `(*Decoder).Decode() (*Message, error)` | Read the next GRIB2 message (returns `io.EOF` when done) |
| `ReadMessage(r io.Reader) (*Message, error)` | Convenience function to read a single message |

### Message Structure

```
Message
  Section0    -- Indicator (discipline, edition, total length)
  Section1    -- Identification (centre, reference time, production status)
  Section2    -- Local Use (optional, centre-specific data)
  Fields[]    -- One or more data fields, each containing:
    Section3  -- Grid Definition (template defining the grid geometry)
    Section4  -- Product Definition (what the data represents)
    Section5  -- Data Representation (packing method and parameters)
    Section6  -- Bitmap (missing data mask)
    Section7  -- Data (packed values)
```

### Field Methods

| Method | Description |
|--------|-------------|
| `Values() ([]float64, error)` | Unpack, apply bitmap, reorder by scanning mode |
| `GridCoordinates() (lats, lons []float64, error)` | Lat/lon of every grid point in data order |
| `NearestValue(lat, lon float64)` | Value at closest grid point (O(1) for regular grids) |
| `BilinearValue(lat, lon float64)` | Bilinear interpolation from 4 surrounding points |

### Writing

| Function | Description |
|----------|-------------|
| `WriteMessage(w io.Writer, msg *Message) error` | Write a complete GRIB2 message (patches total length automatically) |
| `PackSimple(values, bpv)` | Encode with simple packing (Template 5.0) |
| `PackComplex(values, bpv)` | Encode with complex packing (Template 5.2) |
| `PackComplexSpatialDiff(values, bpv, order)` | Encode with complex packing + spatial differencing (Template 5.3) |
| `PackIEEE(values, precision)` | Encode as IEEE 32-bit or 64-bit floats (Template 5.4) |
| `PackJPEG2000(values, bpv)` | Encode with JPEG2000 lossless compression (Template 5.40) |
| `PackPNG(values, bpv, width, height)` | Encode with PNG compression (Template 5.41) |
| `PackCCSDS(values, bpv, blockSize, rsi)` | Encode with CCSDS/AEC compression (Template 5.42) |

### Code Table Lookups

| Function | Table |
|----------|-------|
| `DisciplineName(d uint8)` | Table 0.0 |
| `CentreName(c uint16)` | Common Code Table C-11 |
| `ParameterName(discipline, category, number uint8)` | Table 4.2.x.x |
| `ParameterUnit(discipline, category, number uint8)` | Units for Table 4.2 |
| `ParameterCategoryName(discipline, category uint8)` | Table 4.1.x |
| `SurfaceTypeName(t uint8)` | Table 4.5 |
| `GridDefinitionTemplateName(n uint16)` | Table 3.1 |
| `ProductDefinitionTemplateName(n uint16)` | Table 4.0 |
| `DataRepresentationTemplateName(n uint16)` | Table 5.0 |
| `ReferenceTimeName(t uint8)` | Table 1.2 |
| `ProductionStatusName(s uint8)` | Table 1.3 |
| `DataTypeName(t uint8)` | Table 1.4 |

## Compression Format Details

| Format | Template | Decode | Encode | Notes |
|--------|----------|--------|--------|-------|
| Simple | 5.0 | Yes | Yes | Fast paths for 8/16/32-bit aligned widths; arbitrary bit widths supported |
| Complex | 5.2 | Yes | Yes | Full group splitting with eccodes-compatible merge optimization |
| Complex + Spatial Diff | 5.3 | Yes | Yes | Order 1 and 2 spatial differencing; missing value management |
| IEEE Float | 5.4 | Yes | Yes | 32-bit and 64-bit precision |
| JPEG2000 | 5.40 | Yes | Yes | Pure Go J2K codec (internal/j2k); lossless mode |
| PNG | 5.41 | Yes | Yes | Uses Go standard library `image/png`; 8-bit and 16-bit grayscale |
| CCSDS/AEC | 5.42 | Yes | Yes | Pure Go AEC codec (internal/aec); CCSDS 121.0-B-3 compliant |
| Run-length | 5.200 | Yes | -- | Variable-base run-length decoding with level values |

All decode paths are cross-validated against eccodes reference output at tolerances from 1e-4 to 1e-10 depending on the format.

## Testing

The test suite contains 138 unit tests and 7 fuzz targets covering:

- **Byte-perfect round-trips**: read a GRIB2 file, write it back, compare bytes
- **Cross-validation against eccodes**: decoded values compared to eccodes `grib_get_data` output for every supported packing format (simple, complex, complex+spatial diff, JPEG2000, PNG, CCSDS, IEEE, run-length)
- **Encode/decode round-trips**: pack values, unpack them, verify they match within quantization tolerance
- **Grid coordinate validation**: lat/lon arrays compared against eccodes for regular lat/lon, Mercator, Gaussian, Lambert, Polar Stereographic, and Space View grids
- **Section-level parsing**: individual tests for all section readers and writers (Sections 0-8)
- **Template coverage**: dedicated tests for all grid, product, and data representation templates
- **Fuzz testing**: 7 fuzz targets covering the full message parser and each individual unpacker (JPEG2000, CCSDS, Complex, Run-length, Simple, PNG)
- **Hardening**: all allocations are capped (100M elements max), all bit extractions are bounds-checked, section sizes are limited to 256 MiB

## Performance

- **Pure Go, zero CGo** -- no overhead from C interop, straightforward cross-compilation, no system library requirements
- **Fast paths** for common bit widths (8, 16, 32-bit aligned simple packing)
- **Streaming decoder** -- reads messages one at a time via `Decoder`, suitable for large multi-message files
- **O(1) nearest-neighbour** for regular lat/lon and regular Gaussian grids (direct index computation instead of brute-force search)
- **Fuzz-tested and hardened** -- safe to use on untrusted input; allocation caps and bounds checks prevent resource exhaustion

## License

MIT
