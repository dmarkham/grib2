package j2k

import (
	"os"
	"testing"
)

// FuzzJ2KDecode fuzzes the JPEG 2000 codestream decoder.
// The decoder is the most complex parsing pipeline in the library:
// marker parsing, SIZ/COD/QCD header interpretation, tier-1 MQ arithmetic
// decoding, tier-2 packet parsing, and the inverse DWT.
func FuzzJ2KDecode(f *testing.F) {
	// Seed with a known-good J2K codestream extracted from a GRIB2 file.
	seed, err := os.ReadFile("../../testdata/reference/sample.j2k")
	if err == nil {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Errors are expected on malformed input; panics are bugs.
		Decode(data) //nolint:errcheck
	})
}
