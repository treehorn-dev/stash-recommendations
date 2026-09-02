package model

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"
	"strings"
)

const sceneVectorDimensions = 256

// SceneVector maps canonical catalog features into a fixed, normalized vector.
func SceneVector(features []string) ([]float32, bool) {
	unique := make(map[string]struct{}, len(features))
	for _, feature := range features {
		if feature = strings.TrimSpace(feature); feature != "" {
			unique[feature] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return nil, false
	}

	tokens := make([]string, 0, len(unique))
	for token := range unique {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)

	vector := make([]float32, sceneVectorDimensions)
	for _, token := range tokens {
		digest := sha256.Sum256([]byte(token))
		index := int(binary.BigEndian.Uint16(digest[:2])) % sceneVectorDimensions
		if digest[2]&1 == 0 {
			vector[index]++
		} else {
			vector[index]--
		}
	}

	var squaredLength float64
	for _, value := range vector {
		squaredLength += float64(value * value)
	}
	length := math.Sqrt(squaredLength)
	if length == 0 {
		return nil, false
	}
	for index := range vector {
		vector[index] /= float32(length)
	}
	return vector, true
}
