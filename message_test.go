package grib2

import (
	"bytes"
	"os"
	"testing"
)

func TestReadMessage_SampleFile(t *testing.T) {
	f, err := os.Open("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	msg, err := ReadMessage(f)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if msg.Section0.TotalLength != 1179 {
		t.Errorf("TotalLength = %d, want 1179", msg.Section0.TotalLength)
	}
	if msg.Section1.Centre != 98 {
		t.Errorf("Centre = %d, want 98", msg.Section1.Centre)
	}
	if msg.Section2 == nil {
		t.Fatal("Section2 is nil, want non-nil (sample has section 2)")
	}
	if len(msg.Fields) != 1 {
		t.Fatalf("Fields count = %d, want 1", len(msg.Fields))
	}

	field := msg.Fields[0]
	if field.Section3.NumberOfDataPoints != 496 {
		t.Errorf("NumberOfDataPoints = %d, want 496", field.Section3.NumberOfDataPoints)
	}
	if field.Section5.NumberOfValues != 496 {
		t.Errorf("NumberOfValues = %d, want 496", field.Section5.NumberOfValues)
	}
}

func TestReadMessage_RoundTrip(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.grib2")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	msg, err := ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteMessage(&buf, msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	got := buf.Bytes()
	if !bytes.Equal(got, raw) {
		for i := range got {
			if i >= len(raw) || got[i] != raw[i] {
				t.Fatalf("first diff at byte %d: got 0x%02x, want 0x%02x", i, got[i], raw[i])
			}
		}
		if len(got) != len(raw) {
			t.Fatalf("length mismatch: got %d, want %d", len(got), len(raw))
		}
	}
}

func TestReadMessage_MultiMessage(t *testing.T) {
	// high_level_api.grib2 has 5 separate GRIB2 messages
	f, err := os.Open("testdata/high_level_api.grib2")
	if err != nil {
		t.Skipf("open: %v", err)
	}
	defer f.Close()

	dec := NewDecoder(f)
	var messages []*Message
	for {
		msg, err := dec.Decode()
		if err != nil {
			break
		}
		messages = append(messages, msg)
	}

	if len(messages) != 5 {
		t.Fatalf("message count = %d, want 5", len(messages))
	}

	for i, msg := range messages {
		if len(msg.Fields) != 1 {
			t.Errorf("message %d: field count = %d, want 1", i, len(msg.Fields))
		}
		if msg.Section0.TotalLength != 160219 {
			t.Errorf("message %d: totalLength = %d, want 160219", i, msg.Section0.TotalLength)
		}
	}
}
