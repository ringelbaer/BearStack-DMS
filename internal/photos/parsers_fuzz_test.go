package photos

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"
)

func FuzzEXIF(f *testing.F) {
	for _, seed := range []string{"", "49492a00080000000000", "4d4d002a000000080000", "49492a00ffffffff", "49492a0008000000010012010300010000000600000000000000"} {
		data, _ := hex.DecodeString(seed)
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}
		// Includes malformed IFD counts, offsets, types and byte orders. A parser
		// may reject them, but must never panic or read outside the supplied bytes.
		_, _ = parseEXIF(data)
	})
}

func FuzzGPX(f *testing.F) {
	for _, seed := range []string{"", `<gpx/>`, `<gpx><trkseg><trkpt lat="1" lon="179"/><trkpt lat="2" lon="-179"/></trkseg><trkseg><trkpt lat="NaN" lon="0"/></trkseg></gpx>`, `<gpx><rte><rtept lat="90" lon="180"/></rte></gpx>`, `<gpx><trkpt lat="1"`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32768 {
			t.Skip()
		}
		points, segments, err := decodeGPXSegments(context.Background(), bytes.NewReader(data), 16384, 256)
		if err != nil {
			return
		}
		if len(points) > 256 {
			t.Fatal("point budget exceeded")
		}
		count := 0
		for _, segment := range segments {
			count += len(segment)
			for _, p := range segment {
				if !validGPXPoint(p) {
					t.Fatalf("invalid coordinate: %+v", p)
				}
			}
		}
		if count != len(points) {
			t.Fatal("segments lost or duplicated points")
		}
	})
}

func FuzzXMP(f *testing.F) {
	for _, seed := range []string{"", `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description xmp:Rating="5" xmlns:xmp="http://ns.adobe.com/xap/1.0/"/></rdf:RDF></x:xmpmeta>`, `<rdf:li><mwg-rs:Area x="NaN" y="0.5" w="-1" h="Infinity"/></rdf:li>`, `<!DOCTYPE x [<!ENTITY a "&a;">]><x>&a;</x>`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > 8192 {
			t.Skip()
		}
		_ = parseXMPMetadataWithBase(data, Metadata{})
	})
}
