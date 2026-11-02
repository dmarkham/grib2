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

// FuzzAECRoundTrip encodes fuzzed samples with fuzzed options and requires
// the decoder to give them back whenever the encoder accepts the options
// (libaec fuzzing/fuzz_target.cc does the same round trip).
func FuzzAECRoundTrip(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}, uint8(8), uint8(16), uint16(64), uint8(DataPreprocess|DataMSB))
	f.Add([]byte{255, 0, 255, 0, 1, 1, 1, 1}, uint8(5), uint8(8), uint16(1), uint8(DataSigned|DataPreprocess))
	f.Fuzz(func(t *testing.T, raw []byte, bps, bs uint8, rsi uint16, flags uint8) {
		if len(raw) > 4096 {
			return
		}
		bits := int(bps%32) + 1
		// Standard block sizes only: with AEC_NOT_ENFORCE and blocks over
		// 64 samples libaec cannot decode some of its own streams either
		// (TestNotEnforceLargeBlockMatchesLibaec).
		opts := Options{BitsPerSample: bits, BlockSize: 8 << (bs % 4), RSI: int(rsi%4096) + 1, Flags: uint32(flags) &^ NotEnforce}
		samples := make([]int32, len(raw))
		mask := uint32(1)<<bits - 1
		if bits == 32 {
			mask = ^uint32(0)
		}
		for i, b := range raw {
			v := uint32(b) * 2654435761 & mask
			if opts.Flags&DataSigned != 0 && bits < 32 && v&(1<<(bits-1)) != 0 {
				v |= ^mask // sign-extend
			}
			samples[i] = int32(v)
		}
		enc, err := Encode(samples, opts)
		if err != nil {
			return
		}
		dec, err := Decode(enc, opts)
		if err != nil {
			t.Fatalf("decode of our own stream: %v (opts %+v)", err, opts)
		}
		if len(dec) < len(samples) {
			t.Fatalf("decoded %d samples, encoded %d", len(dec), len(samples))
		}
		for i := range samples {
			if dec[i] != samples[i] {
				t.Fatalf("sample %d: %d, encoded %d (opts %+v)", i, dec[i], samples[i], opts)
			}
		}
	})
}

// TestNotEnforceLargeBlockMatchesLibaec pins a case the round-trip fuzzer
// found: 2 samples, 4 bits, block size 246 (AEC_NOT_ENFORCE), preprocessed.
// libaec 1.1.x encodes exactly these 32 bytes and its own decoder then
// returns AEC_DATA_ERROR; we do the same.
func TestNotEnforceLargeBlockMatchesLibaec(t *testing.T) {
	opts := Options{BitsPerSample: 4, BlockSize: 246, RSI: 36, Flags: DataPreprocess | DataMSB | NotEnforce}
	got, err := Encode([]int32{0, 14}, opts)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xc0}
	if string(got) != string(want) {
		t.Errorf("encoded % x, libaec % x", got, want)
	}
	if _, err := Decode(got, opts); err == nil {
		t.Error("decode succeeded where libaec reports a data error")
	}
}
