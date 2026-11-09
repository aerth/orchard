// Package orderedmap for ordered JSON object keys
package orderedmap

import (
	"bytes"
	"encoding/json"
	"iter"
	"sort"
)

// Item represents a key-value pair to maintain order.
type Item[K comparable, V any] struct {
	Key   K
	Value V
}

// Map is a generic ordered map implementation.
type Map[K comparable, V any] struct {
	keys   []K
	values map[K]V
}

// New creates and initializes a new OrderedMap.
func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		keys:   make([]K, 0),
		values: make(map[K]V),
	}
}

// All returns an iterator over key-value pairs in insertion order (Go 1.23+).
func (m *Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, k := range m.keys {
			if !yield(k, m.values[k]) {
				return
			}
		}
	}
}

// Set adds or updates a key-value pair. If the key is new, it is appended to the order slice.
func (m *Map[K, V]) Set(key K, value V) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

// Get retrieves the value associated with a key and a boolean indicating if it was found.
func (m *Map[K, V]) Get(key K) (V, bool) {
	val, exists := m.values[key]
	return val, exists
}

// Delete removes a key and its value from the map.
func (m *Map[K, V]) Delete(key K) {
	if _, exists := m.values[key]; !exists {
		return
	}
	delete(m.values, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

// Keys returns a slice of keys in insertion order.
func (m *Map[K, V]) Keys() []K {
	return m.keys
}

// Len returns the number of elements in the map.
func (m *Map[K, V]) Len() int {
	return len(m.keys)
}

// MarshalJSON implements the json.Marshaler interface to preserve order.
func (m *Map[K, V]) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')

	encoder := json.NewEncoder(&buf)
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}

		// Marshal the key (must be convertible to string or primitive for JSON objects)
		keyBytes, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')

		// Marshal the value
		err = encoder.Encode(m.values[k])
		if err != nil {
			return nil, err
		}
		buf.Truncate(buf.Len() - 1)
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
//
// TODO: unmarshal ordered
func (m *Map[K, V]) UnmarshalJSON(data []byte) error {
	// Unmarshal into a temporary map to extract keys/values dynamically
	var tempMap map[K]V
	if err := json.Unmarshal(data, &tempMap); err != nil {
		return err
	}

	// Unmarshal raw JSON to preserve original key order from the input stream
	var rawDecoder json.Decoder
	_ = rawDecoder // (Alternative: use Decoder with Token to preserve literal stream order if desired)

	// Simple approach: reset and populate based on standard map unmarshaling,
	// then optionally sort or leave as-is if order from standard map suffices.
	// For strict stream-order unmarshaling, use a json.Decoder with DisallowUnknownFields or tokens.
	m.keys = make([]K, 0, len(tempMap))
	m.values = tempMap

	for k := range tempMap {
		m.keys = append(m.keys, k)
	}

	// Optional: Sort keys if you want deterministic unmarshaled order,
	// otherwise map iteration order is randomized in Go.
	sort.Slice(m.keys, func(i, j int) bool {
		// Generic constraint check would be needed for custom sorting,
		// but standard types can be asserted if required.
		return fmtKey(m.keys[i]) < fmtKey(m.keys[j])
	})

	return nil
}

func fmtKey[K comparable](k K) string {
	// Simple helper to convert comparable keys to string for sorting unmarshaled keys
	b, _ := json.Marshal(k)
	return string(b)
}
