package grib2

import "fmt"

// DisciplineName returns the human-readable name for a GRIB2 discipline
// (Table 0.0: Discipline of processed data).
func DisciplineName(d uint8) string {
	if name, ok := disciplineTable[d]; ok {
		return name
	}
	return fmt.Sprintf("Unknown discipline (%d)", d)
}

// CentreName returns the human-readable name for a WMO originating centre
// (Common Code Table C-11).
func CentreName(c uint16) string {
	if name, ok := centreTable[c]; ok {
		return name
	}
	return fmt.Sprintf("Unknown centre (%d)", c)
}

// ReferenceTimeName returns the human-readable significance of reference time
// (Table 1.2).
func ReferenceTimeName(t uint8) string {
	if name, ok := referenceTimeTable[t]; ok {
		return name
	}
	return fmt.Sprintf("Unknown reference time significance (%d)", t)
}

// ProductionStatusName returns the human-readable production status
// (Table 1.3).
func ProductionStatusName(s uint8) string {
	if name, ok := productionStatusTable[s]; ok {
		return name
	}
	return fmt.Sprintf("Unknown production status (%d)", s)
}

// DataTypeName returns the human-readable type of processed data
// (Table 1.4).
func DataTypeName(t uint8) string {
	if name, ok := dataTypeTable[t]; ok {
		return name
	}
	return fmt.Sprintf("Unknown data type (%d)", t)
}

// GridDefinitionTemplateName returns the human-readable name for a grid
// definition template number (Table 3.1).
func GridDefinitionTemplateName(n uint16) string {
	if name, ok := gridDefinitionTemplateTable[n]; ok {
		return name
	}
	return fmt.Sprintf("Unknown grid definition template (%d)", n)
}

// ProductDefinitionTemplateName returns the human-readable name for a product
// definition template number (Table 4.0).
func ProductDefinitionTemplateName(n uint16) string {
	if name, ok := productDefinitionTemplateTable[n]; ok {
		return name
	}
	return fmt.Sprintf("Unknown product definition template (%d)", n)
}

// ParameterCategoryName returns the human-readable name for a parameter
// category given the discipline (Table 4.1.x).
func ParameterCategoryName(discipline, category uint8) string {
	key := uint16(discipline)<<8 | uint16(category)
	if name, ok := parameterCategoryTable[key]; ok {
		return name
	}
	return fmt.Sprintf("Unknown parameter category (%d, %d)", discipline, category)
}

// ParameterName returns the human-readable name for a parameter given
// discipline, category, and number (Table 4.2.x.x).
func ParameterName(discipline, category, number uint8) string {
	key := uint32(discipline)<<16 | uint32(category)<<8 | uint32(number)
	if name, ok := parameterTable[key]; ok {
		return name
	}
	return fmt.Sprintf("Unknown parameter (%d, %d, %d)", discipline, category, number)
}

// ParameterUnit returns the unit string for a parameter given discipline,
// category, and number.
func ParameterUnit(discipline, category, number uint8) string {
	key := uint32(discipline)<<16 | uint32(category)<<8 | uint32(number)
	if unit, ok := parameterUnitTable[key]; ok {
		return unit
	}
	return "unknown"
}

// SurfaceTypeName returns the human-readable name for a fixed surface type
// (Table 4.5).
func SurfaceTypeName(t uint8) string {
	if name, ok := surfaceTypeTable[t]; ok {
		return name
	}
	return fmt.Sprintf("Unknown surface type (%d)", t)
}

// DataRepresentationTemplateName returns the human-readable name for a data
// representation template number (Table 5.0).
func DataRepresentationTemplateName(n uint16) string {
	if name, ok := dataRepresentationTemplateTable[n]; ok {
		return name
	}
	return fmt.Sprintf("Unknown data representation template (%d)", n)
}

// ---------- Table 0.0: Discipline ----------

var disciplineTable = map[uint8]string{
	0:   "Meteorological products",
	1:   "Hydrological products",
	2:   "Land surface products",
	3:   "Satellite remote sensing products",
	4:   "Space weather products",
	10:  "Oceanographic products",
	20:  "Health and socioeconomic impacts",
	191: "Computational parameters",
	255: "Missing",
}

// ---------- Common Code Table C-11: Originating Centres ----------

var centreTable = map[uint16]string{
	0:   "WMO Secretariat",
	1:   "Melbourne (WMC)",
	4:   "Moscow (WMC)",
	7:   "US National Weather Service - NCEP",
	8:   "US National Weather Service - NWSTG",
	9:   "US National Weather Service - Other",
	10:  "Cairo (RSMC/RAFC)",
	12:  "Dakar (RSMC/RAFC)",
	14:  "Nairobi (RSMC/RAFC)",
	16:  "Casablanca (RSMC)",
	17:  "Tunis (RSMC)",
	24:  "Pretoria (RSMC)",
	25:  "La Reunion (RSMC)",
	26:  "Khabarovsk (RSMC)",
	28:  "New Delhi (IMD)",
	29:  "New Delhi (NCMRWF)",
	30:  "Novosibirsk (RSMC)",
	33:  "Jeddah (RSMC)",
	34:  "Japanese Meteorological Agency - Tokyo (RSMC)",
	38:  "Beijing (RSMC)",
	40:  "Seoul",
	41:  "Buenos Aires (RSMC/RAFC)",
	43:  "Brasilia (RSMC/RAFC)",
	45:  "Santiago",
	46:  "Brazilian Space Agency - INPE",
	51:  "Miami (RSMC/RAFC)",
	52:  "National Hurricane Center, Miami",
	53:  "Canadian Meteorological Service - Montreal (RSMC)",
	54:  "Canadian Meteorological Service - Montreal (RSMC)",
	55:  "San Francisco",
	57:  "U.S. Air Force - Global Weather Center",
	58:  "US Navy - Fleet Numerical Oceanography Center",
	59:  "NOAA Forecast Systems Lab, Boulder CO",
	60:  "National Center for Atmospheric Research (NCAR)",
	65:  "Darwin (RSMC)",
	67:  "Melbourne (RSMC)",
	69:  "Wellington (RSMC/RAFC)",
	74:  "U.K. Met Office - Exeter",
	78:  "Offenbach (DWD)",
	80:  "Rome (RSMC)",
	82:  "Norrkoping (SMHI)",
	84:  "French Weather Service - Toulouse",
	85:  "French Weather Service - Toulouse",
	86:  "Helsinki (FMI)",
	88:  "Oslo (NMI)",
	94:  "Copenhagen (DMI)",
	97:  "European Space Agency (ESA)",
	98:  "European Centre for Medium-Range Weather Forecasts (ECMWF)",
	99:  "DeBilt, Netherlands (KNMI)",
	110: "Hong Kong",
	160: "US NOAA/NESDIS",
	161: "US NOAA Office of Oceanic and Atmospheric Research",
	173: "US National Aeronautics and Space Administration (NASA)",
	195: "Indonesia (NMC)",
	210: "Frascati (ESA/ESRIN)",
	214: "INM Madrid",
	215: "Zurich (MeteoSwiss)",
	224: "Vienna (ZAMG)",
	227: "Belgium (NMC)",
	233: "Dublin",
	235: "INGV",
	247: "OPERA - EUMETNET",
	250: "COSMO",
	252: "Max Planck Institute for Meteorology (MPI-M)",
	254: "EUMETSAT Operation Centre",
}

// ---------- Table 1.2: Significance of Reference Time ----------

var referenceTimeTable = map[uint8]string{
	0:   "Analysis",
	1:   "Start of forecast",
	2:   "Verifying time of forecast",
	3:   "Observation time",
	4:   "Local time",
	255: "Missing",
}

// ---------- Table 1.3: Production Status of Data ----------

var productionStatusTable = map[uint8]string{
	0:   "Operational products",
	1:   "Operational test products",
	2:   "Research products",
	3:   "Re-analysis products",
	4:   "THORPEX Interactive Grand Global Ensemble (TIGGE)",
	5:   "THORPEX Interactive Grand Global Ensemble test (TIGGE)",
	6:   "S2S operational products",
	7:   "S2S test products",
	8:   "Uncertainties in Ensembles of Regional ReAnalyses (UERRA)",
	9:   "Uncertainties in Ensembles of Regional ReAnalyses test (UERRA)",
	10:  "Copernicus regional reanalysis (CARRA/CERRA)",
	11:  "Copernicus regional reanalysis test (CARRA/CERRA)",
	255: "Missing",
}

// ---------- Table 1.4: Type of Data ----------

var dataTypeTable = map[uint8]string{
	0:   "Analysis products",
	1:   "Forecast products",
	2:   "Analysis and forecast products",
	3:   "Control forecast products",
	4:   "Perturbed forecast products",
	5:   "Control and perturbed forecast products",
	6:   "Processed satellite observations",
	7:   "Processed radar observations",
	8:   "Event probability",
	255: "Missing",
}

// ---------- Table 3.1: Grid Definition Template Number ----------

var gridDefinitionTemplateTable = map[uint16]string{
	0:     "Latitude/longitude (equidistant cylindrical, or Plate Carree)",
	1:     "Rotated latitude/longitude",
	2:     "Stretched latitude/longitude",
	3:     "Stretched and rotated latitude/longitude",
	4:     "Variable resolution latitude/longitude",
	5:     "Variable resolution rotated latitude/longitude",
	10:    "Mercator",
	12:    "Transverse Mercator",
	20:    "Polar stereographic projection",
	30:    "Lambert conformal",
	31:    "Albers equal area",
	40:    "Gaussian latitude/longitude",
	41:    "Rotated Gaussian latitude/longitude",
	42:    "Stretched Gaussian latitude/longitude",
	43:    "Stretched and rotated Gaussian latitude/longitude",
	50:    "Spherical harmonic coefficients",
	90:    "Space view perspective or orthographic",
	100:   "Triangular grid based on an icosahedron",
	101:   "General unstructured grid",
	110:   "Equatorial azimuthal equidistant projection",
	120:   "Azimuth-range projection",
	140:   "Lambert azimuthal equal area projection",
	1000:  "Cross-section grid with points equally spaced on the horizontal",
	1100:  "Hovmoller diagram grid",
	1200:  "Time section grid",
	65535: "Missing",
}

// ---------- Table 4.0: Product Definition Template Number ----------

var productDefinitionTemplateTable = map[uint16]string{
	0:     "Analysis or forecast at a horizontal level at a point in time",
	1:     "Individual ensemble forecast at a horizontal level at a point in time",
	2:     "Derived forecasts based on all ensemble members at a horizontal level at a point in time",
	3:     "Derived forecasts based on a cluster of ensemble members over a rectangular area",
	4:     "Derived forecasts based on a cluster of ensemble members over a circular area",
	5:     "Probability forecasts at a horizontal level at a point in time",
	6:     "Percentile forecasts at a horizontal level at a point in time",
	7:     "Analysis or forecast error at a horizontal level at a point in time",
	8:     "Average, accumulation, or extreme values in a continuous or non-continuous time interval",
	9:     "Probability forecasts in a continuous or non-continuous time interval",
	10:    "Percentile forecasts in a continuous or non-continuous time interval",
	11:    "Individual ensemble forecast in a continuous or non-continuous time interval",
	12:    "Derived forecasts based on all ensemble members in a continuous or non-continuous time interval",
	15:    "Average, accumulation, or extreme values over a spatial area at a point in time",
	20:    "Radar product",
	31:    "Satellite product",
	32:    "Analysis or forecast for simulated (synthetic) satellite data",
	40:    "Analysis or forecast for atmospheric chemical constituents",
	41:    "Individual ensemble forecast for atmospheric chemical constituents",
	42:    "Average/accumulation for atmospheric chemical constituents in a time interval",
	43:    "Individual ensemble forecast for atmospheric chemical constituents in a time interval",
	44:    "Analysis or forecast for aerosol",
	45:    "Individual ensemble forecast for aerosol",
	46:    "Average/accumulation for aerosol in a time interval",
	48:    "Analysis or forecast for optical properties of aerosol",
	51:    "Categorical forecasts at a horizontal level at a point in time",
	53:    "Partitioned parameters at a horizontal level at a point in time",
	55:    "Spatio-temporal changing tiles at a horizontal level at a point in time",
	60:    "Individual ensemble reforecast at a horizontal level at a point in time",
	61:    "Individual ensemble reforecast in a continuous or non-continuous time interval",
	70:    "Post-processing analysis or forecast at a horizontal level at a point in time",
	71:    "Post-processing individual ensemble forecast at a horizontal level at a point in time",
	72:    "Post-processing average/accumulation/extreme values in a time interval",
	73:    "Post-processing individual ensemble forecast in a time interval",
	254:   "CCITT IA5 character string",
	1000:  "Cross-section of analysis and forecast at a point in time",
	1001:  "Cross-section of averaged or statistically processed analysis or forecast",
	1100:  "Hovmoller-type grid with no averaging or other statistical processing",
	1101:  "Hovmoller-type grid with averaging or other statistical processing",
	65535: "Missing",
}

// ---------- Table 4.1.x: Parameter Category ----------
// Key: (discipline << 8) | category

var parameterCategoryTable = map[uint16]string{
	// Discipline 0 - Meteorological
	0x0000: "Temperature",
	0x0001: "Moisture",
	0x0002: "Momentum",
	0x0003: "Mass",
	0x0004: "Short-wave radiation",
	0x0005: "Long-wave radiation",
	0x0006: "Cloud",
	0x0007: "Thermodynamic stability indices",
	0x0008: "Kinematic stability indices",
	0x0009: "Temperature probabilities",
	0x000A: "Moisture probabilities",
	0x000B: "Momentum probabilities",
	0x000C: "Mass probabilities",
	0x000D: "Aerosols",
	0x000E: "Trace gases",
	0x000F: "Radar",
	0x0010: "Forecast radar imagery",
	0x0011: "Electrodynamics",
	0x0012: "Nuclear/radiology",
	0x0013: "Physical atmospheric properties",
	0x0014: "Atmospheric chemical constituents",
	0x00BF: "Miscellaneous",

	// Discipline 1 - Hydrological
	0x0100: "Hydrology basic products",
	0x0101: "Hydrology probabilities",
	0x0102: "Inland water and sediment properties",

	// Discipline 2 - Land surface
	0x0200: "Vegetation/biomass",
	0x0201: "Agricultural/aquacultural special products",
	0x0202: "Transportation-related products",
	0x0203: "Soil products",
	0x0204: "Fire weather products",
	0x0205: "Glaciers and inland ice",

	// Discipline 3 - Satellite remote sensing
	0x0300: "Image format products",
	0x0301: "Quantitative products",
	0x0302: "Cloud properties",
	0x0303: "Flight rule conditions",
	0x0304: "Volcanic ash",
	0x0305: "Sea-surface temperature",
	0x0306: "Solar radiation",

	// Discipline 4 - Space weather
	0x0400: "Temperature",
	0x0401: "Momentum",
	0x0402: "Charged particle mass and number",
	0x0403: "Electric and magnetic fields",
	0x0404: "Energetic particles",
	0x0405: "Waves",
	0x0406: "Solar electromagnetic emissions",
	0x0407: "Terrestrial electromagnetic emissions",
	0x0408: "Imagery",
	0x0409: "Ion-neutral coupling",
	0x040A: "Space weather indices",

	// Discipline 10 - Oceanographic
	0x0A00: "Waves",
	0x0A01: "Currents",
	0x0A02: "Ice",
	0x0A03: "Surface properties",
	0x0A04: "Subsurface properties",
	0x0ABF: "Miscellaneous",

	// Discipline 20 - Health and socioeconomic impacts
	0x1400: "Health indicators",
	0x1401: "Epidemiology",
	0x1402: "Socioeconomic indicators",
}

// ---------- Table 4.2.x.x: Parameter Number ----------
// Key: (discipline << 16) | (category << 8) | number
//
// parameterTable holds the parameter name.
// parameterUnitTable holds the unit string.

var parameterTable = map[uint32]string{
	// ---- Discipline 0, Category 0: Temperature ----
	0x000000: "Temperature",
	0x000001: "Virtual temperature",
	0x000002: "Potential temperature",
	0x000003: "Pseudo-adiabatic potential temperature",
	0x000004: "Maximum temperature",
	0x000005: "Minimum temperature",
	0x000006: "Dewpoint temperature",
	0x000007: "Dewpoint depression",
	0x000008: "Lapse rate",
	0x000009: "Temperature anomaly",
	0x00000A: "Latent heat net flux",
	0x00000B: "Sensible heat net flux",
	0x00000C: "Heat index",
	0x00000D: "Wind chill factor",
	0x00000E: "Minimum dewpoint depression",
	0x00000F: "Virtual potential temperature",
	0x000010: "Snow phase change heat flux",
	0x000011: "Skin temperature",
	0x000012: "Snow temperature (top of snow)",
	0x000015: "Apparent temperature",
	0x00001B: "Wet-bulb temperature",
	0x00001D: "Temperature advection",
	0x000020: "Wet-bulb potential temperature",

	// ---- Discipline 0, Category 1: Moisture ----
	0x000100: "Specific humidity",
	0x000101: "Relative humidity",
	0x000102: "Humidity mixing ratio",
	0x000103: "Precipitable water",
	0x000104: "Vapour pressure",
	0x000105: "Saturation deficit",
	0x000106: "Evaporation",
	0x000107: "Precipitation rate",
	0x000108: "Total precipitation",
	0x000109: "Large-scale precipitation (non-convective)",
	0x00010A: "Convective precipitation",
	0x00010B: "Snow depth",
	0x00010C: "Snowfall rate water equivalent",
	0x00010D: "Water equivalent of accumulated snow depth",
	0x00010E: "Convective snow",
	0x00010F: "Large-scale snow",
	0x000110: "Snow melt",
	0x000112: "Absolute humidity",
	0x000113: "Precipitation type",
	0x000114: "Integrated liquid water",
	0x000115: "Condensate",
	0x000116: "Cloud mixing ratio",
	0x000117: "Ice water mixing ratio",
	0x000118: "Rain mixing ratio",
	0x000119: "Snow mixing ratio",
	0x00011A: "Horizontal moisture convergence",
	0x00011B: "Maximum relative humidity",
	0x00011D: "Total snowfall",
	0x00011F: "Hail",
	0x000120: "Graupel (snow pellets)",
	0x000121: "Categorical rain",
	0x000122: "Categorical freezing rain",
	0x000123: "Categorical ice pellets",
	0x000124: "Categorical snow",
	0x000125: "Convective precipitation rate",
	0x000128: "Potential evaporation",
	0x000129: "Potential evaporation rate",
	0x00012A: "Snow cover",
	0x000134: "Total column integrated water vapour",
	0x000140: "Total column integrated cloud water",
	0x000141: "Rain precipitation rate",
	0x000142: "Snow precipitation rate",
	0x000143: "Freezing rain precipitation rate",
	0x000144: "Ice pellets precipitation rate",
	0x000145: "Total column integrated cloud ice",
	0x000150: "Total condensate",
	0x000151: "Total column-integrated condensate",
	0x000153: "Specific cloud liquid water content",
	0x000154: "Specific cloud ice water content",
	0x000155: "Specific rainwater content",
	0x000156: "Specific snow water content",

	// ---- Discipline 0, Category 2: Momentum ----
	0x000200: "Wind direction (from which blowing)",
	0x000201: "Wind speed",
	0x000202: "u-component of wind",
	0x000203: "v-component of wind",
	0x000204: "Stream function",
	0x000205: "Velocity potential",
	0x000206: "Montgomery stream function",
	0x000207: "Sigma coordinate vertical velocity",
	0x000208: "Vertical velocity (pressure)",
	0x000209: "Vertical velocity (geometric)",
	0x00020A: "Absolute vorticity",
	0x00020B: "Absolute divergence",
	0x00020C: "Relative vorticity",
	0x00020D: "Relative divergence",
	0x00020E: "Potential vorticity",
	0x00020F: "Vertical u-component shear",
	0x000210: "Vertical v-component shear",
	0x000211: "Momentum flux, u-component",
	0x000212: "Momentum flux, v-component",
	0x000213: "Wind mixing energy",
	0x000214: "Boundary layer dissipation",
	0x000215: "Maximum wind speed",
	0x000216: "Wind speed (gust)",
	0x000217: "u-component of wind (gust)",
	0x000218: "v-component of wind (gust)",
	0x000219: "Vertical speed shear",
	0x00021B: "u-component storm motion",
	0x00021C: "v-component storm motion",
	0x00021D: "Drag coefficient",
	0x00021E: "Frictional velocity",
	0x00021F: "Turbulent diffusion coefficient for momentum",
	0x000229: "u-component of geostrophic wind",
	0x00022A: "v-component of geostrophic wind",

	// ---- Discipline 0, Category 3: Mass ----
	0x000300: "Pressure",
	0x000301: "Pressure reduced to MSL",
	0x000302: "Pressure tendency",
	0x000303: "ICAO Standard Atmosphere reference height",
	0x000304: "Geopotential",
	0x000305: "Geopotential height",
	0x000306: "Geometric height",
	0x000307: "Standard deviation of height",
	0x000308: "Pressure anomaly",
	0x000309: "Geopotential height anomaly",
	0x00030A: "Density",
	0x00030B: "Altimeter setting",
	0x00030C: "Thickness",
	0x00030D: "Pressure altitude",
	0x00030E: "Density altitude",
	0x00030F: "5-wave geopotential height",
	0x000310: "Zonal flux of gravity wave stress",
	0x000311: "Meridional flux of gravity wave stress",
	0x000312: "Planetary boundary layer height",
	0x000313: "5-wave geopotential height anomaly",
	0x000314: "Standard deviation of sub-grid scale orography",
	0x000319: "Natural logarithm of pressure in Pa",

	// ---- Discipline 0, Category 4: Short-wave radiation ----
	0x000400: "Net short-wave radiation flux (surface)",
	0x000401: "Net short-wave radiation flux (top of atmosphere)",
	0x000402: "Short-wave radiation flux",
	0x000403: "Global radiation flux",
	0x000404: "Brightness temperature",
	0x000407: "Downward short-wave radiation flux",
	0x000408: "Upward short-wave radiation flux",
	0x000409: "Net short wave radiation flux",
	0x00040A: "Photosynthetically active radiation",
	0x00040B: "Net short-wave radiation flux, clear sky",
	0x00040D: "Direct short-wave radiation flux",
	0x00040E: "Diffuse short-wave radiation flux",

	// ---- Discipline 0, Category 5: Long-wave radiation ----
	0x000500: "Net long-wave radiation flux (surface)",
	0x000501: "Net long-wave radiation flux (top of atmosphere)",
	0x000502: "Long-wave radiation flux",
	0x000503: "Downward long-wave radiation flux",
	0x000504: "Upward long-wave radiation flux",
	0x000505: "Net long-wave radiation flux",
	0x000506: "Net long-wave radiation flux, clear sky",
	0x000507: "Brightness temperature (long-wave)",
	0x000508: "Downward long-wave radiation flux, clear sky",

	// ---- Discipline 0, Category 6: Cloud ----
	0x000600: "Cloud ice",
	0x000601: "Total cloud cover",
	0x000602: "Convective cloud cover",
	0x000603: "Low cloud cover",
	0x000604: "Medium cloud cover",
	0x000605: "High cloud cover",
	0x000606: "Cloud water",
	0x000607: "Cloud amount",
	0x000608: "Cloud type",
	0x000609: "Thunderstorm maximum tops",
	0x00060A: "Thunderstorm coverage",
	0x00060B: "Cloud base",
	0x00060C: "Cloud top",
	0x00060D: "Ceiling",
	0x00060E: "Non-convective cloud cover",
	0x00060F: "Cloud work function",
	0x000610: "Convective cloud efficiency",
	0x000611: "Total condensate",
	0x000612: "Total column-integrated cloud water",
	0x000613: "Total column-integrated cloud ice",
	0x000614: "Total column-integrated condensate",
	0x000615: "Ice fraction of total condensate",
	0x000616: "Cloud cover",
	0x000617: "Cloud ice mixing ratio",
	0x000618: "Sunshine",
	0x000621: "Sunshine duration",
	0x000632: "Fog",

	// ---- Discipline 0, Category 7: Thermodynamic stability ----
	0x000700: "Parcel lifted index (to 500 hPa)",
	0x000701: "Best lifted index (to 500 hPa)",
	0x000702: "K index",
	0x000703: "KO index",
	0x000704: "Total totals index",
	0x000705: "Sweat index",
	0x000706: "Convective available potential energy",
	0x000707: "Convective inhibition",
	0x000708: "Storm relative helicity",
	0x000709: "Energy helicity index",
	0x00070A: "Surface lifted index",
	0x00070B: "Best (4-layer) lifted index",
	0x00070C: "Richardson number",
	0x00070D: "Showalter index",
	0x00070F: "Updraught helicity",

	// ---- Discipline 0, Category 13: Aerosols ----
	0x000D00: "Aerosol type",

	// ---- Discipline 0, Category 14: Trace gases ----
	0x000E00: "Total ozone",
	0x000E01: "Ozone mixing ratio",
	0x000E02: "Total column integrated ozone",

	// ---- Discipline 0, Category 15: Radar ----
	0x000F00: "Base spectrum width",
	0x000F01: "Base reflectivity",
	0x000F02: "Base radial velocity",
	0x000F03: "Vertically integrated liquid water (VIL)",
	0x000F04: "Layer-maximum base reflectivity",
	0x000F05: "Precipitation",

	// ---- Discipline 0, Category 16: Forecast radar imagery ----
	0x001000: "Equivalent radar reflectivity factor for rain",
	0x001001: "Equivalent radar reflectivity factor for snow",
	0x001002: "Equivalent radar reflectivity factor for parameterized convection",
	0x001003: "Echo top",
	0x001004: "Reflectivity",
	0x001005: "Composite reflectivity",

	// ---- Discipline 0, Category 17: Electrodynamics ----
	0x001100: "Lightning strike density",
	0x001101: "Lightning potential index (LPI)",

	// ---- Discipline 0, Category 19: Physical atmospheric properties ----
	0x001300: "Visibility",
	0x001301: "Albedo",
	0x001302: "Thunderstorm probability",
	0x001303: "Mixed layer depth",
	0x001304: "Volcanic ash",
	0x001305: "Icing top",
	0x001306: "Icing base",
	0x001307: "Icing",
	0x001308: "Turbulence top",
	0x001309: "Turbulence base",
	0x00130A: "Turbulence",
	0x00130B: "Turbulent kinetic energy",
	0x001310: "Maximum snow albedo",
	0x001311: "Snow free albedo",
	0x001312: "Snow albedo",
	0x001314: "In-cloud turbulence",
	0x001315: "Clear air turbulence (CAT)",
	0x001320: "Highest freezing level",
	// CMC local use parameters (non-WMO, widely used in Canadian models)
	0x001326: "Sky transparency index",
	0x001327: "Seeing index",

	// ---- Discipline 0, Category 20: Atmospheric chemical constituents ----
	0x001400: "Mass density (concentration)",
	0x001401: "Column-integrated mass density",
	0x001402: "Mass mixing ratio (mass fraction in air)",
	0x001403: "Atmosphere emission mass flux",
	0x001464: "Aerosol optical thickness",

	// ---- Discipline 1, Category 0: Hydrology basic ----
	0x010000: "Flash flood guidance",
	0x010001: "Flash flood runoff",
	0x010002: "Remotely-sensed snow cover",
	0x010003: "Elevation of snow-covered terrain",
	0x010004: "Snow water equivalent percent of normal",
	0x010005: "Baseflow-groundwater runoff",
	0x010006: "Storm surface runoff",
	0x010007: "Discharge from rivers or streams",

	// ---- Discipline 1, Category 1: Hydrology probabilities ----
	0x010100: "Conditional percent precipitation amount fractile",
	0x010101: "Percent precipitation in a sub-period",
	0x010102: "Probability of 0.01 inch of precipitation (POP)",

	// ---- Discipline 1, Category 2: Inland water and sediment ----
	0x010200: "Water depth",
	0x010201: "Water temperature",
	0x010202: "Water fraction",

	// ---- Discipline 2, Category 0: Vegetation/biomass ----
	0x020000: "Land cover (0=sea, 1=land)",
	0x020001: "Surface roughness",
	0x020002: "Soil temperature",
	0x020003: "Soil moisture content",
	0x020004: "Vegetation",
	0x020005: "Water runoff",
	0x020006: "Evapotranspiration",
	0x020007: "Model terrain height",
	0x020008: "Land use",
	0x020009: "Volumetric soil moisture content",
	0x02000A: "Ground heat flux",
	0x02000B: "Moisture availability",
	0x02000D: "Plant canopy surface water",
	0x02001C: "Leaf area index",
	0x02001F: "Normalized differential vegetation index (NDVI)",

	// ---- Discipline 2, Category 3: Soil products ----
	0x020300: "Soil type",
	0x020301: "Upper layer soil temperature",
	0x020302: "Upper layer soil moisture",
	0x020303: "Lower layer soil moisture",
	0x020304: "Bottom layer soil temperature",
	0x020312: "Soil temperature",
	0x020313: "Soil moisture",

	// ---- Discipline 2, Category 4: Fire weather ----
	0x020400: "Fire outlook",
	0x020405: "Forest Fire Weather Index (FWI)",

	// ---- Discipline 10, Category 0: Waves ----
	0x0A0000: "Wave spectra (1)",
	0x0A0003: "Significant height of combined wind waves and swell",
	0x0A0004: "Direction of wind waves",
	0x0A0005: "Significant height of wind waves",
	0x0A0006: "Mean period of wind waves",
	0x0A0007: "Direction of swell waves",
	0x0A0008: "Significant height of swell waves",
	0x0A0009: "Mean period of swell waves",
	0x0A000A: "Primary wave direction",
	0x0A000B: "Primary wave mean period",
	0x0A000E: "Mean direction of combined wind waves and swell",
	0x0A000F: "Mean period of combined wind waves and swell",
	0x0A0015: "u-component surface Stokes drift",
	0x0A0016: "v-component surface Stokes drift",
	0x0A0018: "Maximum individual wave height",
	0x0A0022: "Peak wave period",
	0x0A0025: "Altimeter wave height",

	// ---- Discipline 10, Category 1: Currents ----
	0x0A0100: "Current direction",
	0x0A0101: "Current speed",
	0x0A0102: "u-component of current",
	0x0A0103: "v-component of current",

	// ---- Discipline 10, Category 2: Ice ----
	0x0A0200: "Ice cover",
	0x0A0201: "Ice thickness",
	0x0A0202: "Direction of ice drift",
	0x0A0203: "Speed of ice drift",
	0x0A0204: "u-component of ice drift",
	0x0A0205: "v-component of ice drift",
	0x0A0208: "Ice temperature",

	// ---- Discipline 10, Category 3: Surface properties ----
	0x0A0300: "Water temperature",
	0x0A0301: "Deviation of sea level from mean",
	0x0A0303: "Practical salinity",

	// ---- Discipline 10, Category 4: Subsurface properties ----
	0x0A0400: "Main thermocline depth",
	0x0A0403: "Salinity",
	0x0A0407: "Bathymetry",
	0x0A040F: "Water temperature",
	0x0A0410: "Water density (rho)",
}

var parameterUnitTable = map[uint32]string{
	// ---- Discipline 0, Category 0: Temperature ----
	0x000000: "K",
	0x000001: "K",
	0x000002: "K",
	0x000003: "K",
	0x000004: "K",
	0x000005: "K",
	0x000006: "K",
	0x000007: "K",
	0x000008: "K/m",
	0x000009: "K",
	0x00000A: "W m-2",
	0x00000B: "W m-2",
	0x00000C: "K",
	0x00000D: "K",
	0x00000E: "K",
	0x00000F: "K",
	0x000010: "W m-2",
	0x000011: "K",
	0x000012: "K",
	0x000015: "K",
	0x00001B: "K",
	0x00001D: "K s-1",
	0x000020: "K",

	// ---- Discipline 0, Category 1: Moisture ----
	0x000100: "kg/kg",
	0x000101: "%",
	0x000102: "kg/kg",
	0x000103: "kg m-2",
	0x000104: "Pa",
	0x000105: "Pa",
	0x000106: "kg m-2",
	0x000107: "kg m-2 s-1",
	0x000108: "kg m-2",
	0x000109: "kg m-2",
	0x00010A: "kg m-2",
	0x00010B: "m",
	0x00010C: "kg m-2 s-1",
	0x00010D: "kg m-2",
	0x00010E: "kg m-2",
	0x00010F: "kg m-2",
	0x000110: "kg m-2",
	0x000112: "kg m-3",
	0x000114: "kg m-2",
	0x000115: "kg/kg",
	0x000116: "kg/kg",
	0x000117: "kg/kg",
	0x000118: "kg/kg",
	0x000119: "kg/kg",
	0x00011A: "kg kg-1 s-1",
	0x00011B: "%",
	0x00011D: "m",
	0x00011F: "m",
	0x000120: "kg/kg",
	0x000125: "kg m-2 s-1",
	0x000128: "kg m-2",
	0x000129: "W m-2",
	0x00012A: "%",
	0x000134: "kg m-2",
	0x000140: "kg m-2",
	0x000141: "kg m-2 s-1",
	0x000142: "kg m-2 s-1",
	0x000143: "kg m-2 s-1",
	0x000144: "kg m-2 s-1",
	0x000145: "kg m-2",
	0x000150: "kg/kg",
	0x000151: "kg m-2",
	0x000153: "kg/kg",
	0x000154: "kg/kg",
	0x000155: "kg/kg",
	0x000156: "kg/kg",

	// ---- Discipline 0, Category 2: Momentum ----
	0x000200: "degree true",
	0x000201: "m/s",
	0x000202: "m/s",
	0x000203: "m/s",
	0x000204: "m2/s",
	0x000205: "m2/s",
	0x000206: "m2 s-2",
	0x000207: "/s",
	0x000208: "Pa/s",
	0x000209: "m/s",
	0x00020A: "/s",
	0x00020B: "/s",
	0x00020C: "/s",
	0x00020D: "/s",
	0x00020E: "K m2 kg-1 s-1",
	0x00020F: "/s",
	0x000210: "/s",
	0x000211: "N m-2",
	0x000212: "N m-2",
	0x000213: "J",
	0x000214: "W m-2",
	0x000215: "m/s",
	0x000216: "m/s",
	0x000217: "m/s",
	0x000218: "m/s",
	0x000219: "/s",
	0x00021B: "m/s",
	0x00021C: "m/s",
	0x00021E: "m/s",
	0x00021F: "m2/s",
	0x000229: "m/s",
	0x00022A: "m/s",

	// ---- Discipline 0, Category 3: Mass ----
	0x000300: "Pa",
	0x000301: "Pa",
	0x000302: "Pa/s",
	0x000303: "m",
	0x000304: "m2 s-2",
	0x000305: "gpm",
	0x000306: "m",
	0x000307: "m",
	0x000308: "Pa",
	0x000309: "gpm",
	0x00030A: "kg m-3",
	0x00030B: "Pa",
	0x00030C: "m",
	0x00030D: "m",
	0x00030E: "m",
	0x00030F: "gpm",
	0x000310: "N m-2",
	0x000311: "N m-2",
	0x000312: "m",
	0x000313: "gpm",
	0x000314: "m",

	// ---- Discipline 0, Category 4: Short-wave radiation ----
	0x000400: "W m-2",
	0x000401: "W m-2",
	0x000402: "W m-2",
	0x000403: "W m-2",
	0x000404: "K",
	0x000407: "W m-2",
	0x000408: "W m-2",
	0x000409: "W m-2",
	0x00040A: "W m-2",
	0x00040B: "W m-2",
	0x00040D: "W m-2",
	0x00040E: "W m-2",

	// ---- Discipline 0, Category 5: Long-wave radiation ----
	0x000500: "W m-2",
	0x000501: "W m-2",
	0x000502: "W m-2",
	0x000503: "W m-2",
	0x000504: "W m-2",
	0x000505: "W m-2",
	0x000506: "W m-2",
	0x000507: "K",
	0x000508: "W m-2",

	// ---- Discipline 0, Category 6: Cloud ----
	0x000600: "kg m-2",
	0x000601: "%",
	0x000602: "%",
	0x000603: "%",
	0x000604: "%",
	0x000605: "%",
	0x000606: "kg m-2",
	0x000607: "%",
	0x000609: "m",
	0x00060B: "m",
	0x00060C: "m",
	0x00060D: "m",
	0x00060E: "%",
	0x00060F: "J/kg",
	0x000612: "kg m-2",
	0x000613: "kg m-2",
	0x000614: "kg m-2",
	0x000616: "%",
	0x000617: "kg/kg",
	0x000621: "s",
	0x000632: "%",

	// ---- Discipline 0, Category 7: Thermodynamic stability ----
	0x000700: "K",
	0x000701: "K",
	0x000702: "K",
	0x000703: "K",
	0x000704: "K",
	0x000706: "J/kg",
	0x000707: "J/kg",
	0x000708: "J/kg",
	0x00070A: "K",
	0x00070B: "K",
	0x00070D: "K",
	0x00070F: "m2 s-2",

	// ---- Discipline 0, Category 14: Trace gases ----
	0x000E00: "DU",
	0x000E01: "kg/kg",
	0x000E02: "DU",

	// ---- Discipline 0, Category 19: Physical atmospheric properties ----
	0x001300: "m",
	0x001301: "%",
	0x001302: "%",
	0x001303: "m",
	0x00130B: "J/kg",
	0x001320: "m",
	// CMC local use
	0x001326: "Numeric",
	0x001327: "Numeric",

	// ---- Discipline 2, Category 0: Vegetation/biomass ----
	0x020001: "m",
	0x020002: "K",
	0x020003: "kg m-2",
	0x020004: "%",
	0x020005: "kg m-2",
	0x020006: "kg-2 s-1",
	0x020007: "m",
	0x02000A: "W m-2",
	0x02000B: "%",
	0x02000D: "kg m-2",

	// ---- Discipline 10, Category 0: Waves ----
	0x0A0003: "m",
	0x0A0004: "degree true",
	0x0A0005: "m",
	0x0A0006: "s",
	0x0A0007: "degree true",
	0x0A0008: "m",
	0x0A0009: "s",
	0x0A000A: "degree true",
	0x0A000B: "s",
	0x0A000E: "degree true",
	0x0A000F: "s",
	0x0A0015: "m/s",
	0x0A0016: "m/s",
	0x0A0018: "m",
	0x0A0022: "s",
	0x0A0025: "m",

	// ---- Discipline 10, Category 1: Currents ----
	0x0A0100: "degree true",
	0x0A0101: "m/s",
	0x0A0102: "m/s",
	0x0A0103: "m/s",

	// ---- Discipline 10, Category 3: Surface properties ----
	0x0A0300: "K",
	0x0A0301: "m",
}

// ---------- Table 4.5: Fixed Surface Types ----------

var surfaceTypeTable = map[uint8]string{
	1:   "Ground or water surface",
	2:   "Cloud base level",
	3:   "Level of cloud tops",
	4:   "Level of 0 degree C isotherm",
	5:   "Level of adiabatic condensation lifted from the surface",
	6:   "Maximum wind level",
	7:   "Tropopause",
	8:   "Nominal top of the atmosphere",
	9:   "Sea bottom",
	10:  "Entire atmosphere",
	11:  "Cumulonimbus (CB) base",
	12:  "Cumulonimbus (CB) top",
	20:  "Isothermal level",
	100: "Isobaric surface",
	101: "Mean sea level",
	102: "Specific altitude above mean sea level",
	103: "Specified height level above ground",
	104: "Sigma level",
	105: "Hybrid level",
	106: "Depth below land surface",
	107: "Isentropic (theta) level",
	108: "Level at specified pressure difference from ground to level",
	109: "Potential vorticity surface",
	111: "Eta level",
	113: "Logarithmic hybrid level",
	114: "Snow level",
	117: "Mixed layer depth",
	118: "Hybrid height level",
	119: "Hybrid pressure level",
	150: "Generalized vertical height coordinate",
	151: "Soil level",
	160: "Depth below sea level",
	161: "Depth below water surface",
	162: "Lake or river bottom",
	166: "Mixing layer",
	174: "Top surface of ice on sea, lake or river",
	177: "Deep soil (of indefinite depth)",
	200: "Entire atmosphere (considered as a single layer)",
	204: "Highest tropospheric freezing level",
	211: "Boundary layer cloud layer",
	212: "Low cloud bottom level",
	213: "Low cloud top level",
	214: "Low cloud layer",
	222: "Middle cloud bottom level",
	223: "Middle cloud top level",
	224: "Middle cloud layer",
	232: "High cloud bottom level",
	233: "High cloud top level",
	234: "High cloud layer",
	242: "Convective cloud bottom level",
	243: "Convective cloud top level",
	244: "Convective cloud layer",
	255: "Missing",
}

// ---------- Table 5.0: Data Representation Template Number ----------

var dataRepresentationTemplateTable = map[uint16]string{
	0:     "Grid point data - simple packing",
	1:     "Matrix value at grid point - simple packing",
	2:     "Grid point data - complex packing",
	3:     "Grid point data - complex packing and spatial differencing",
	4:     "Grid point data - IEEE floating point data",
	40:    "Grid point data - JPEG 2000 code stream format",
	41:    "Grid point data - Portable Network Graphics (PNG)",
	42:    "Grid point data - CCSDS recommended lossless compression",
	50:    "Spectral data - simple packing",
	51:    "Spherical harmonics data - complex packing",
	61:    "Grid point data - simple packing with logarithm pre-processing",
	200:   "Run length packing with level values",
	65535: "Missing",
}

// String returns a human-readable description of Section0.
func (s Section0) String() string {
	return fmt.Sprintf("Section0{Discipline: %d (%s), Edition: %d, TotalLength: %d}",
		s.Discipline, DisciplineName(s.Discipline), s.Edition, s.TotalLength)
}

// String returns a human-readable description of Section1.
func (s Section1) String() string {
	return fmt.Sprintf("Section1{Centre: %d (%s), RefTime: %s, Status: %s, Type: %s}",
		s.Centre, CentreName(s.Centre),
		s.ReferenceTime().Format("2006-01-02T15:04:05Z"),
		ProductionStatusName(s.ProductionStatus),
		DataTypeName(s.TypeOfData))
}

// String returns a human-readable description of Section3.
func (s Section3) String() string {
	return fmt.Sprintf("Section3{Template: %d (%s), DataPoints: %d}",
		s.TemplateNumber, GridDefinitionTemplateName(s.TemplateNumber),
		s.NumberOfDataPoints)
}

// String returns a human-readable description of Section4.
func (s Section4) String() string {
	desc := fmt.Sprintf("Section4{Template: %d (%s)",
		s.TemplateNumber, ProductDefinitionTemplateName(s.TemplateNumber))
	if tmpl, ok := s.Template.(Template40); ok {
		desc += fmt.Sprintf(", Parameter: %s [%s]",
			ParameterName(0, tmpl.ParameterCategory, tmpl.ParameterNumber),
			ParameterUnit(0, tmpl.ParameterCategory, tmpl.ParameterNumber))
		desc += fmt.Sprintf(", Surface: %s",
			SurfaceTypeName(tmpl.TypeOfFirstFixedSurface))
	}
	if tmpl, ok := s.Template.(Template41); ok {
		desc += fmt.Sprintf(", Parameter: %s [%s]",
			ParameterName(0, tmpl.ParameterCategory, tmpl.ParameterNumber),
			ParameterUnit(0, tmpl.ParameterCategory, tmpl.ParameterNumber))
		desc += fmt.Sprintf(", Surface: %s",
			SurfaceTypeName(tmpl.TypeOfFirstFixedSurface))
	}
	desc += "}"
	return desc
}

// String returns a human-readable description of Section5.
func (s Section5) String() string {
	return fmt.Sprintf("Section5{Template: %d (%s), Values: %d}",
		s.TemplateNumber, DataRepresentationTemplateName(s.TemplateNumber),
		s.NumberOfValues)
}
