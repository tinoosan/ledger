package meta

import (
	"encoding/json"
	"testing"
)

func TestSetGetDelMergeClone(t *testing.T) {
	metaMap := New(nil)
	metaMap.Set("a", "1")
	if value, ok := metaMap.Get("a"); !ok || value != "1" {
		t.Fatalf("get failed")
	}
	metaToMerge := New(map[string]string{"b": "2"})
	metaMap.Merge(metaToMerge)
	if value, ok := metaMap.Get("b"); !ok || value != "2" {
		t.Fatalf("merge failed")
	}
	cloned := metaMap.Clone()
	if len(cloned) != 2 || cloned["a"] != "1" {
		t.Fatalf("clone failed: %+v", cloned)
	}
	metaMap.Del("a")
	if _, ok := metaMap.Get("a"); ok {
		t.Fatalf("del failed")
	}
}

func TestValidationLimits(t *testing.T) {
	// too many pairs
	pairs := make(map[string]string)
	for i := 0; i < MaxPairs+1; i++ {
		pairs[string('a'+byte(i%26))+"k"+string('a'+byte(i%26))] = "v"
	}
	metaMap := New(pairs)
	if err := metaMap.Validate(); err == nil {
		t.Fatalf("expected too many pairs")
	}
	// key too long
	longKey := make([]byte, MaxKeyLen+1)
	for i := range longKey {
		longKey[i] = 'k'
	}
	metaMap = New(map[string]string{string(longKey): "v"})
	if err := metaMap.Validate(); err == nil {
		t.Fatalf("expected key too long")
	}
	// value too long
	longVal := make([]byte, MaxValLen+1)
	for i := range longVal {
		longVal[i] = 'v'
	}
	metaMap = New(map[string]string{"k": string(longVal)})
	if err := metaMap.Validate(); err == nil {
		t.Fatalf("expected value too long")
	}
}

func TestStableJSONAndRoundtrip(t *testing.T) {
	metaMap := New(map[string]string{"b": "2", "a": "1"})
	b1, _ := metaMap.MarshalStableJSON()
	if string(b1) != `{"a":"1","b":"2"}` {
		t.Fatalf("unexpected stable json: %s", string(b1))
	}
	var unmarshaled Metadata
	if err := json.Unmarshal(b1, &unmarshaled); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := unmarshaled.Validate(); err != nil {
		t.Fatalf("validate roundtrip: %v", err)
	}
}

// SQL adapters removed; no tests needed.
