package aec

import (
	"testing"
)

// FuzzAECDecode fuzzes the AEC/CCSDS-121.0-B-3 decoder state machine.
// The decoder processes a compressed bitstream using Golomb-Rice coding,
// second extension, zero-block, and uncompressed modes. Malformed input
// can trigger edge cases in the bit I/O, run-length calculations, and
// post-processing (preprocessing undo).
func FuzzAECDecode(f *testing.F) {
	// Generate a seed by encoding known data with the AEC encoder.
	// This gives us a valid compressed bitstream to start from.
	samples := make([]int32, 256)
	for i := range samples {
		samples[i] = int32(i * 7 % 1000)
	}
	opts := Options{
		BitsPerSample: 16,
		BlockSize:     32,
		RSI:           128,
		Flags:         DataPreprocess,
	}
	encoded, err := Encode(samples, opts)
	if err == nil {
		f.Add(encoded)
	}

	// Also seed with a small synthetic stream: all zeros compressed.
	zeros := make([]int32, 128)
	encodedZeros, err := Encode(zeros, Options{
		BitsPerSample: 8,
		BlockSize:     16,
		RSI:           64,
		Flags:         DataPreprocess,
	})
	if err == nil {
		f.Add(encodedZeros)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Use the same options as the first seed. The fuzzer will mutate
		// the compressed data, not the options.
		Decode(data, opts) //nolint:errcheck
	})
}

// FuzzAECDecodeVariousOptions fuzzes with different valid option combinations
// to exercise more decoder paths.
func FuzzAECDecodeVariousOptions(f *testing.F) {
	samples := make([]int32, 64)
	for i := range samples {
		samples[i] = int32(i)
	}

	// 8-bit, no preprocessing
	opts8 := Options{BitsPerSample: 8, BlockSize: 8, RSI: 16, Flags: 0}
	if enc, err := Encode(samples, opts8); err == nil {
		f.Add(enc, uint8(8), uint8(8), uint8(16), false)
	}

	// 16-bit, with preprocessing
	opts16 := Options{BitsPerSample: 16, BlockSize: 16, RSI: 32, Flags: DataPreprocess}
	if enc, err := Encode(samples, opts16); err == nil {
		f.Add(enc, uint8(16), uint8(16), uint8(32), true)
	}

	// 32-bit, with preprocessing
	opts32 := Options{BitsPerSample: 32, BlockSize: 32, RSI: 64, Flags: DataPreprocess}
	if enc, err := Encode(samples, opts32); err == nil {
		f.Add(enc, uint8(32), uint8(32), uint8(64), true)
	}

	f.Fuzz(func(t *testing.T, data []byte, bps uint8, blockSz uint8, rsi uint8, preprocess bool) {
		// Clamp to valid ranges
		bitsPerSample := int(bps)
		if bitsPerSample < 1 || bitsPerSample > 32 {
			return
		}
		bs := int(blockSz)
		if bs < 2 || bs > 64 || bs%2 != 0 {
			return
		}
		r := int(rsi)
		if r < 1 || r > 255 {
			return
		}

		var flags uint32
		if preprocess {
			flags = DataPreprocess
		}

		opts := Options{
			BitsPerSample: bitsPerSample,
			BlockSize:     bs,
			RSI:           r,
			Flags:         flags,
		}

		Decode(data, opts) //nolint:errcheck
	})
}
