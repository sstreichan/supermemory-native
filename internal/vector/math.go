package vector

import (
	"errors"
	"math"
)

// DotProduct calculates the dot product of two vectors.
// Accumulation happens in float64: with 1000+ dimensions float32 summation
// loses ranking-relevant precision.
func DotProduct(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, errors.New("vector dimensions must be equal")
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return float32(dot), nil
}

// L2Norm calculates the L2 norm (magnitude) of a vector, accumulated in float64.
func L2Norm(a []float32) float32 {
	var sum float64
	for _, val := range a {
		sum += float64(val) * float64(val)
	}
	return float32(math.Sqrt(sum))
}

// CosineSimilarity calculates the cosine similarity between two vectors
func CosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, errors.New("vector dimensions must be equal")
	}
	dot, err := DotProduct(a, b)
	if err != nil {
		return 0, err
	}
	normA := L2Norm(a)
	normB := L2Norm(b)
	if normA == 0 || normB == 0 {
		return 0, nil // Avoid division by zero
	}
	return dot / (normA * normB), nil
}
