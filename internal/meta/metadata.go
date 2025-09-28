package meta

import (
    "bytes"
    "encoding/json"
    "errors"
    "sort"
)

// Metadata is a small string map with validation and stable JSON encoding.
type Metadata map[string]string

const (
    MaxPairs     = 20
    MaxKeyLen    = 64
    MaxValLen    = 256
    MaxTotalJSON = 4096
)

func New(m map[string]string) Metadata {
    if m == nil { return Metadata{} }
    out := make(Metadata, len(m))
    for k, v := range m { out[k] = v }
    return out
}

func (m Metadata) Clone() Metadata {
    if m == nil { return Metadata{} }
    out := make(Metadata, len(m))
    for k, v := range m { out[k] = v }
    return out
}

func (m Metadata) Get(k string) (string, bool) { v, ok := m[k]; return v, ok }

func (m Metadata) Set(k, v string) {
    if len(m) >= MaxPairs {
        // drop if exceeding pair limit; caller should Validate() to detect
        return
    }
    if len(k) == 0 || len(k) > MaxKeyLen { return }
    if len(v) > MaxValLen { return }
    m[k] = v
}

func (m Metadata) Del(k string) { delete(m, k) }

func (m Metadata) Merge(other Metadata) {
    if other == nil { return }
    // deterministic order merge by keys
    keys := make([]string, 0, len(other))
    for k := range other { keys = append(keys, k) }
    sort.Strings(keys)
    for _, k := range keys { m.Set(k, other[k]) }
}

func (m Metadata) Validate() error {
    if len(m) > MaxPairs { return errors.New("metadata too many pairs") }
    for k, v := range m {
        if len(k) == 0 || len(k) > MaxKeyLen { return errors.New("metadata key too long or empty") }
        if len(v) > MaxValLen { return errors.New("metadata value too long") }
    }
    b, err := m.MarshalStableJSON()
    if err != nil { return err }
    if len(b) > MaxTotalJSON { return errors.New("metadata exceeds max json size") }
    return nil
}

// MarshalStableJSON returns a deterministic JSON representation with keys sorted.
func (m Metadata) MarshalStableJSON() ([]byte, error) {
    if m == nil || len(m) == 0 { return []byte("{}"), nil }
    keys := make([]string, 0, len(m))
    for k := range m { keys = append(keys, k) }
    sort.Strings(keys)
    buf := &bytes.Buffer{}
    buf.WriteByte('{')
    for i, k := range keys {
        kb, _ := json.Marshal(k)
        vb, _ := json.Marshal(m[k])
        buf.Write(kb)
        buf.WriteByte(':')
        buf.Write(vb)
        if i < len(keys)-1 { buf.WriteByte(',') }
    }
    buf.WriteByte('}')
    return buf.Bytes(), nil
}

// JSON marshal/unmarshal use stable encoding
func (m Metadata) MarshalJSON() ([]byte, error) { return m.MarshalStableJSON() }

func (m *Metadata) UnmarshalJSON(b []byte) error {
    var tmp map[string]string
    if len(b) == 0 || bytes.Equal(b, []byte("null")) { *m = Metadata{}; return nil }
    if err := json.Unmarshal(b, &tmp); err != nil { return err }
    *m = New(tmp)
    return nil
}
