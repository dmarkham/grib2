package grib2

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// metaKey reads one eccodes key from our decoded field. width is the octet
// width of the field, used to compare eccodes' MISSING with the all-ones
// value; ok is false when the field has no such key.
type metaKey struct {
	name  string
	width int
	get   func(m *Message, f *Field) (v int64, ok bool)
}

func productKeys() []metaKey {
	t40 := func(f *Field) *Template40 { return f.extractTemplate40() }
	timeRange := func(f *Field) []TimeRangeSpec {
		switch t := f.Section4.Template.(type) {
		case Template48:
			return t.TimeRangeSpecs
		case Template411:
			return t.TimeRangeSpecs
		case Template412:
			return t.TimeRangeSpecs
		}
		return nil
	}
	sm := func(v int8) int64 { return int64(v) }
	return []metaKey{
		{"discipline", 1, func(m *Message, f *Field) (int64, bool) { return int64(m.Section0.Discipline), true }},
		{"centre", 2, func(m *Message, f *Field) (int64, bool) { return int64(m.Section1.Centre), true }},
		{"section1Length", 4, func(m *Message, f *Field) (int64, bool) { return int64(m.Section1.Length), true }},
		{"dataDate", 0, func(m *Message, f *Field) (int64, bool) {
			s := m.Section1
			return int64(s.Year)*10000 + int64(s.Month)*100 + int64(s.Day), true
		}},
		{"dataTime", 0, func(m *Message, f *Field) (int64, bool) {
			return int64(m.Section1.Hour)*100 + int64(m.Section1.Minute), true
		}},
		{"numberOfDataPoints", 4, func(m *Message, f *Field) (int64, bool) { return int64(f.Section3.NumberOfDataPoints), true }},
		{"gridDefinitionTemplateNumber", 2, func(m *Message, f *Field) (int64, bool) { return int64(f.Section3.TemplateNumber), true }},
		{"productDefinitionTemplateNumber", 2, func(m *Message, f *Field) (int64, bool) { return int64(f.Section4.TemplateNumber), true }},
		{"NV", 2, func(m *Message, f *Field) (int64, bool) { return int64(f.Section4.NV), true }},
		{"section4Length", 4, func(m *Message, f *Field) (int64, bool) { return int64(f.Section4.Length), true }},
		{"dataRepresentationTemplateNumber", 2, func(m *Message, f *Field) (int64, bool) { return int64(f.Section5.TemplateNumber), true }},
		{"numberOfValues", 4, func(m *Message, f *Field) (int64, bool) { return int64(f.Section5.NumberOfValues), true }},
		{"bitMapIndicator", 1, func(m *Message, f *Field) (int64, bool) { return int64(f.Section6.Indicator), true }},
		{"parameterCategory", 1, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return int64(t.ParameterCategory), true
			}
			return 0, false
		}},
		{"parameterNumber", 1, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return int64(t.ParameterNumber), true
			}
			return 0, false
		}},
		{"indicatorOfUnitOfTimeRange", 1, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return int64(t.IndicatorOfUnitOfTimeRange), true
			}
			return 0, false
		}},
		{"forecastTime", 4, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return int64(t.ForecastTime), true
			}
			return 0, false
		}},
		{"typeOfFirstFixedSurface", 1, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return int64(t.TypeOfFirstFixedSurface), true
			}
			return 0, false
		}},
		{"scaleFactorOfFirstFixedSurface", -1, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return sm(t.ScaleFactorOfFirstSurface), true
			}
			return 0, false
		}},
		{"scaledValueOfFirstFixedSurface", 4, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return int64(t.ScaledValueOfFirstSurface), true
			}
			return 0, false
		}},
		{"typeOfSecondFixedSurface", 1, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return int64(t.TypeOfSecondFixedSurface), true
			}
			return 0, false
		}},
		{"scaleFactorOfSecondFixedSurface", -1, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return sm(t.ScaleFactorOfSecondSurface), true
			}
			return 0, false
		}},
		{"scaledValueOfSecondFixedSurface", 4, func(m *Message, f *Field) (int64, bool) {
			if t := t40(f); t != nil {
				return int64(t.ScaledValueOfSecondSurface), true
			}
			return 0, false
		}},
		{"perturbationNumber", 1, func(m *Message, f *Field) (int64, bool) {
			switch t := f.Section4.Template.(type) {
			case Template41:
				return int64(t.PerturbationNumber), true
			case Template411:
				return int64(t.PerturbationNumber), true
			}
			return 0, false
		}},
		{"numberOfForecastsInEnsemble", 1, func(m *Message, f *Field) (int64, bool) {
			switch t := f.Section4.Template.(type) {
			case Template41:
				return int64(t.NumberOfForecastsInEnsemble), true
			case Template42:
				return int64(t.NumberOfForecastsInEnsemble), true
			case Template411:
				return int64(t.NumberOfForecastsInEnsemble), true
			case Template412:
				return int64(t.NumberOfForecastsInEnsemble), true
			}
			return 0, false
		}},
		{"numberOfTimeRange", 1, func(m *Message, f *Field) (int64, bool) {
			if tr := timeRange(f); tr != nil {
				return int64(len(tr)), true
			}
			return 0, false
		}},
		{"lengthOfTimeRange", 4, func(m *Message, f *Field) (int64, bool) {
			if tr := timeRange(f); len(tr) > 0 {
				return int64(tr[len(tr)-1].LengthOfTimeRange), true // eccodes reports the last
			}
			return 0, false
		}},
		{"typeOfStatisticalProcessing", 1, func(m *Message, f *Field) (int64, bool) {
			if tr := timeRange(f); len(tr) > 0 {
				return int64(tr[len(tr)-1].TypeOfStatisticalProcessing), true
			}
			return 0, false
		}},
	}
}

// TestEccodes_ProductMetadata compares identification, product and
// representation keys with eccodes for every field of every fixture
// (eccodes tests/grib_step.sh, grib_sub_hourly.sh, grib_neg_fctime.sh,
// grib2_templates.sh, grib_level.sh).
func TestEccodes_ProductMetadata(t *testing.T) {
	if !eccodesAvailable(t) {
		t.Skip("eccodes not available")
	}
	keys := productKeys()
	var spec []string
	for _, k := range keys {
		spec = append(spec, k.name+":i")
	}
	for _, p := range allFixturePaths(t) {
		t.Run(p, func(t *testing.T) {
			var fields []*Field
			var msgs []*Message
			for _, m := range decodeAll(t, p) {
				for i := range m.Fields {
					fields = append(fields, &m.Fields[i])
					msgs = append(msgs, m)
				}
			}
			cmd := exec.Command(filepath.Join("reference", "eccodes-install", "bin", "grib_get"), "-f", "-p", strings.Join(spec, ","), p)
			cmd.Env = eccodesEnv()
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if strings.Contains(stderr.String(), "ECCODES ERROR") {
				if _, known := decodeExpectedErrors[p]; !known {
					t.Fatalf("grib_get reported: %s", stderr.String())
				}
			}
			if strings.TrimSpace(string(out)) == "" {
				if _, known := decodeExpectedErrors[p]; known {
					t.Skip("eccodes cannot open this file either")
				}
				t.Fatalf("grib_get printed nothing: %v", err)
			}
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) != len(fields) {
				t.Fatalf("eccodes lists %d fields, we decode %d", len(lines), len(fields))
			}
			for i, line := range lines {
				vals := strings.Fields(line)
				if len(vals) != len(keys) {
					t.Fatalf("field %d: %d eccodes values for %d keys: %q", i, len(vals), len(keys), line)
				}
				for k, key := range keys {
					ours, ok := key.get(msgs[i], fields[i])
					ev := vals[k]
					switch {
					case ev == "not_found":
						continue // eccodes has no such key for this template
					case !ok:
						if _, raw := fields[i].Section4.Template.(*RawProductTemplate); raw {
							continue // we keep unparsed templates raw
						}
						t.Errorf("field %d: eccodes has %s=%s, we have no value", i, key.name, ev)
					case ev == "MISSING":
						if !isAllOnes(ours, key.width) {
							t.Errorf("field %d: %s = %d, eccodes MISSING", i, key.name, ours)
						}
					default:
						e, err := strconv.ParseInt(ev, 10, 64)
						if err != nil {
							t.Fatalf("field %d: %s=%q", i, key.name, ev)
						}
						if e != ours {
							t.Errorf("field %d: %s = %d, eccodes %d", i, key.name, ours, e)
						}
					}
				}
			}
		})
	}
}

// isAllOnes reports whether v is the missing value of a width-octet field;
// width -1 is a sign-magnitude octet (missing reads as -127).
func isAllOnes(v int64, width int) bool {
	switch width {
	case -1:
		return v == int64(MissingScaleFactor)
	case 1:
		return v == 0xFF
	case 2:
		return v == 0xFFFF
	case 4:
		return v == 0xFFFFFFFF
	}
	return false
}
