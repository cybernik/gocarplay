package protocol

import (
	"testing"
)

func TestUnmarshalHeader(t *testing.T) {
	data := []byte{170, 85, 170, 85, 116, 14, 0, 0, 6, 0, 0, 0, 249, 255, 255, 255}
	var hdr Header
	var hdr2 Header
	err := Unmarshal(data, &hdr)
	if err != nil {
		t.Fatal(err)
	}
	hdr2, err = UnmarshalHeader(data)
	if err != nil {
		t.Fatal(err)
	}
	if hdr != hdr2 {
		t.Fatal("Headers not equal")
	}
}

func BenchmarkUnmarshalHeader(b *testing.B) {
	data := []byte{170, 85, 170, 85, 116, 14, 0, 0, 6, 0, 0, 0, 249, 255, 255, 255}
	var hdr Header
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := Unmarshal(data, &hdr)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalHeaderNew(b *testing.B) {
	data := []byte{170, 85, 170, 85, 116, 14, 0, 0, 6, 0, 0, 0, 249, 255, 255, 255}
	var err error
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = UnmarshalHeader(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
