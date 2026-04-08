package grib2

import (
	"strings"
	"testing"
)

func TestCodeTableDisciplineName(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{0, "Meteorological products"},
		{1, "Hydrological products"},
		{2, "Land surface products"},
		{3, "Satellite remote sensing products"},
		{4, "Space weather products"},
		{10, "Oceanographic products"},
		{20, "Health and socioeconomic impacts"},
		{191, "Computational parameters"},
		{255, "Missing"},
	}
	for _, tt := range tests {
		got := DisciplineName(tt.code)
		if got != tt.want {
			t.Errorf("DisciplineName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}

	// Unknown discipline should contain "Unknown"
	got := DisciplineName(99)
	if !strings.Contains(got, "Unknown") {
		t.Errorf("DisciplineName(99) = %q, expected to contain 'Unknown'", got)
	}
}

func TestCodeTableCentreName(t *testing.T) {
	tests := []struct {
		code uint16
		want string
	}{
		{7, "US National Weather Service - NCEP"},
		{34, "Japanese Meteorological Agency - Tokyo (RSMC)"},
		{74, "U.K. Met Office - Exeter"},
		{78, "Offenbach (DWD)"},
		{85, "French Weather Service - Toulouse"},
		{98, "European Centre for Medium-Range Weather Forecasts (ECMWF)"},
		{38, "Beijing (RSMC)"},
		{54, "Canadian Meteorological Service - Montreal (RSMC)"},
		{160, "US NOAA/NESDIS"},
		{254, "EUMETSAT Operation Centre"},
	}
	for _, tt := range tests {
		got := CentreName(tt.code)
		if got != tt.want {
			t.Errorf("CentreName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}

	got := CentreName(9999)
	if !strings.Contains(got, "Unknown") {
		t.Errorf("CentreName(9999) = %q, expected to contain 'Unknown'", got)
	}
}

func TestCodeTableReferenceTimeName(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{0, "Analysis"},
		{1, "Start of forecast"},
		{2, "Verifying time of forecast"},
		{3, "Observation time"},
		{255, "Missing"},
	}
	for _, tt := range tests {
		got := ReferenceTimeName(tt.code)
		if got != tt.want {
			t.Errorf("ReferenceTimeName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeTableProductionStatusName(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{0, "Operational products"},
		{1, "Operational test products"},
		{2, "Research products"},
		{3, "Re-analysis products"},
	}
	for _, tt := range tests {
		got := ProductionStatusName(tt.code)
		if got != tt.want {
			t.Errorf("ProductionStatusName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeTableDataTypeName(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{0, "Analysis products"},
		{1, "Forecast products"},
		{2, "Analysis and forecast products"},
		{3, "Control forecast products"},
		{4, "Perturbed forecast products"},
	}
	for _, tt := range tests {
		got := DataTypeName(tt.code)
		if got != tt.want {
			t.Errorf("DataTypeName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeTableGridDefinitionTemplateName(t *testing.T) {
	tests := []struct {
		code uint16
		want string
	}{
		{0, "Latitude/longitude (equidistant cylindrical, or Plate Carree)"},
		{1, "Rotated latitude/longitude"},
		{10, "Mercator"},
		{20, "Polar stereographic projection"},
		{30, "Lambert conformal"},
		{40, "Gaussian latitude/longitude"},
		{90, "Space view perspective or orthographic"},
		{140, "Lambert azimuthal equal area projection"},
	}
	for _, tt := range tests {
		got := GridDefinitionTemplateName(tt.code)
		if got != tt.want {
			t.Errorf("GridDefinitionTemplateName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeTableProductDefinitionTemplateName(t *testing.T) {
	tests := []struct {
		code uint16
		want string
	}{
		{0, "Analysis or forecast at a horizontal level at a point in time"},
		{1, "Individual ensemble forecast at a horizontal level at a point in time"},
		{8, "Average, accumulation, or extreme values in a continuous or non-continuous time interval"},
		{11, "Individual ensemble forecast in a continuous or non-continuous time interval"},
	}
	for _, tt := range tests {
		got := ProductDefinitionTemplateName(tt.code)
		if got != tt.want {
			t.Errorf("ProductDefinitionTemplateName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeTableParameterCategoryName(t *testing.T) {
	tests := []struct {
		disc uint8
		cat  uint8
		want string
	}{
		{0, 0, "Temperature"},
		{0, 1, "Moisture"},
		{0, 2, "Momentum"},
		{0, 3, "Mass"},
		{0, 4, "Short-wave radiation"},
		{0, 5, "Long-wave radiation"},
		{0, 6, "Cloud"},
		{0, 7, "Thermodynamic stability indices"},
		{0, 14, "Trace gases"},
		{0, 19, "Physical atmospheric properties"},
		{1, 0, "Hydrology basic products"},
		{2, 0, "Vegetation/biomass"},
		{2, 3, "Soil products"},
		{10, 0, "Waves"},
		{10, 1, "Currents"},
		{10, 2, "Ice"},
		{10, 3, "Surface properties"},
	}
	for _, tt := range tests {
		got := ParameterCategoryName(tt.disc, tt.cat)
		if got != tt.want {
			t.Errorf("ParameterCategoryName(%d, %d) = %q, want %q", tt.disc, tt.cat, got, tt.want)
		}
	}
}

func TestCodeTableParameterName(t *testing.T) {
	tests := []struct {
		disc uint8
		cat  uint8
		num  uint8
		want string
	}{
		// Meteorological - Temperature
		{0, 0, 0, "Temperature"},
		{0, 0, 2, "Potential temperature"},
		{0, 0, 4, "Maximum temperature"},
		{0, 0, 5, "Minimum temperature"},
		{0, 0, 6, "Dewpoint temperature"},
		{0, 0, 7, "Dewpoint depression"},
		{0, 0, 17, "Skin temperature"},

		// Meteorological - Moisture
		{0, 1, 0, "Specific humidity"},
		{0, 1, 1, "Relative humidity"},
		{0, 1, 3, "Precipitable water"},
		{0, 1, 7, "Precipitation rate"},
		{0, 1, 8, "Total precipitation"},
		{0, 1, 10, "Convective precipitation"},
		{0, 1, 11, "Snow depth"},
		{0, 1, 13, "Water equivalent of accumulated snow depth"},
		{0, 1, 42, "Snow cover"},

		// Meteorological - Momentum
		{0, 2, 0, "Wind direction (from which blowing)"},
		{0, 2, 1, "Wind speed"},
		{0, 2, 2, "u-component of wind"},
		{0, 2, 3, "v-component of wind"},
		{0, 2, 8, "Vertical velocity (pressure)"},
		{0, 2, 10, "Absolute vorticity"},
		{0, 2, 22, "Wind speed (gust)"},

		// Meteorological - Mass
		{0, 3, 0, "Pressure"},
		{0, 3, 1, "Pressure reduced to MSL"},
		{0, 3, 4, "Geopotential"},
		{0, 3, 5, "Geopotential height"},
		{0, 3, 6, "Geometric height"},
		{0, 3, 18, "Planetary boundary layer height"},

		// Meteorological - Cloud
		{0, 6, 1, "Total cloud cover"},
		{0, 6, 3, "Low cloud cover"},
		{0, 6, 4, "Medium cloud cover"},
		{0, 6, 5, "High cloud cover"},

		// Meteorological - Stability
		{0, 7, 6, "Convective available potential energy"},
		{0, 7, 7, "Convective inhibition"},

		// Meteorological - Radiation
		{0, 4, 7, "Downward short-wave radiation flux"},
		{0, 5, 3, "Downward long-wave radiation flux"},

		// Meteorological - Trace gases
		{0, 14, 0, "Total ozone"},

		// Meteorological - Physical atmospheric properties
		{0, 19, 0, "Visibility"},
		{0, 19, 1, "Albedo"},

		// Ocean - Waves
		{10, 0, 3, "Significant height of combined wind waves and swell"},
		{10, 0, 34, "Peak wave period"},

		// Ocean - Surface
		{10, 3, 0, "Water temperature"},

		// Land surface
		{2, 0, 0, "Land cover (0=sea, 1=land)"},
		{2, 0, 2, "Soil temperature"},

		// Hydrology
		{1, 0, 7, "Discharge from rivers or streams"},
	}
	for _, tt := range tests {
		got := ParameterName(tt.disc, tt.cat, tt.num)
		if got != tt.want {
			t.Errorf("ParameterName(%d, %d, %d) = %q, want %q",
				tt.disc, tt.cat, tt.num, got, tt.want)
		}
	}

	// Unknown parameter should contain "Unknown"
	got := ParameterName(0, 0, 254)
	if !strings.Contains(got, "Unknown") {
		t.Errorf("ParameterName(0, 0, 254) = %q, expected to contain 'Unknown'", got)
	}
}

func TestCodeTableParameterUnit(t *testing.T) {
	tests := []struct {
		disc uint8
		cat  uint8
		num  uint8
		want string
	}{
		{0, 0, 0, "K"},           // Temperature
		{0, 1, 1, "%"},           // Relative humidity
		{0, 1, 8, "kg m-2"},     // Total precipitation
		{0, 2, 1, "m/s"},        // Wind speed
		{0, 2, 2, "m/s"},        // u-wind
		{0, 3, 0, "Pa"},         // Pressure
		{0, 3, 5, "gpm"},        // Geopotential height
		{0, 6, 1, "%"},          // Total cloud cover
		{0, 7, 6, "J/kg"},       // CAPE
		{10, 0, 3, "m"},         // Significant wave height
	}
	for _, tt := range tests {
		got := ParameterUnit(tt.disc, tt.cat, tt.num)
		if got != tt.want {
			t.Errorf("ParameterUnit(%d, %d, %d) = %q, want %q",
				tt.disc, tt.cat, tt.num, got, tt.want)
		}
	}

	// Unknown should return "unknown"
	got := ParameterUnit(0, 0, 254)
	if got != "unknown" {
		t.Errorf("ParameterUnit(0, 0, 254) = %q, want %q", got, "unknown")
	}
}

func TestCodeTableSurfaceTypeName(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{1, "Ground or water surface"},
		{2, "Cloud base level"},
		{3, "Level of cloud tops"},
		{7, "Tropopause"},
		{8, "Nominal top of the atmosphere"},
		{10, "Entire atmosphere"},
		{100, "Isobaric surface"},
		{101, "Mean sea level"},
		{103, "Specified height level above ground"},
		{105, "Hybrid level"},
		{106, "Depth below land surface"},
		{107, "Isentropic (theta) level"},
		{200, "Entire atmosphere (considered as a single layer)"},
		{255, "Missing"},
	}
	for _, tt := range tests {
		got := SurfaceTypeName(tt.code)
		if got != tt.want {
			t.Errorf("SurfaceTypeName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeTableDataRepresentationTemplateName(t *testing.T) {
	tests := []struct {
		code uint16
		want string
	}{
		{0, "Grid point data - simple packing"},
		{2, "Grid point data - complex packing"},
		{3, "Grid point data - complex packing and spatial differencing"},
		{40, "Grid point data - JPEG 2000 code stream format"},
		{41, "Grid point data - Portable Network Graphics (PNG)"},
		{42, "Grid point data - CCSDS recommended lossless compression"},
	}
	for _, tt := range tests {
		got := DataRepresentationTemplateName(tt.code)
		if got != tt.want {
			t.Errorf("DataRepresentationTemplateName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestCodeTableSection0String(t *testing.T) {
	s := Section0{Discipline: 0, Edition: 2, TotalLength: 1179}
	got := s.String()
	if !strings.Contains(got, "Meteorological") {
		t.Errorf("Section0.String() = %q, expected to contain 'Meteorological'", got)
	}
	if !strings.Contains(got, "1179") {
		t.Errorf("Section0.String() = %q, expected to contain '1179'", got)
	}
}

func TestCodeTableSection1String(t *testing.T) {
	s := Section1{
		Centre:           7,
		Year:             2024,
		Month:            6,
		Day:              15,
		Hour:             12,
		ProductionStatus: 0,
		TypeOfData:       1,
	}
	got := s.String()
	if !strings.Contains(got, "NCEP") {
		t.Errorf("Section1.String() = %q, expected to contain 'NCEP'", got)
	}
	if !strings.Contains(got, "Forecast") {
		t.Errorf("Section1.String() = %q, expected to contain 'Forecast'", got)
	}
}

func TestCodeTableSection5String(t *testing.T) {
	s := Section5{
		TemplateNumber: 40,
		NumberOfValues: 100000,
	}
	got := s.String()
	if !strings.Contains(got, "JPEG 2000") {
		t.Errorf("Section5.String() = %q, expected to contain 'JPEG 2000'", got)
	}
}
