package db

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// encodeVector serializes a vector into a compact little-endian float32 BLOB.
// An empty (but non-nil) slice is stored for nil input so the NOT NULL
// constraint on the column is never violated.
func encodeVector(v []float32) []byte {
	if v == nil {
		return []byte{}
	}
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// decodeVector deserializes a stored vector BLOB written by encodeVector.
// Legacy JSON text rows are converted at database-open time (see NewSqliteDB),
// so only the binary representation is expected here.
func decodeVector(raw []byte) ([]float32, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw)%4 != 0 {
		// Defensive: a leftover legacy JSON row that migration could not parse.
		var v []float32
		if err := json.Unmarshal(raw, &v); err == nil {
			return v, nil
		}
		return nil, fmt.Errorf("invalid vector storage: length %d is not a multiple of 4", len(raw))
	}
	v := make([]float32, len(raw)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return v, nil
}

// vectorNorm returns the L2 norm of the vector, accumulated in float64 for
// numerical stability across high-dimensional embeddings.
func vectorNorm(v []float32) float64 {
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	return math.Sqrt(sum)
}
