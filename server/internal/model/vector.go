package model

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

const sceneVectorDimensions = 256

type WeightedInteraction struct {
	ContentKey domain.ContentKey
	Weight     float64
}

func vectorLiteral(vector []float32) string {
	values := make([]string, len(vector))
	for index, value := range vector {
		values[index] = strconv.FormatFloat(float64(value), 'f', -1, 32)
	}
	return "[" + strings.Join(values, ",") + "]"
}

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

// ProfileVector builds one normalized account profile and records consumed scenes.
func ProfileVector(sceneVectors map[domain.ContentKey][]float32, interactions []WeightedInteraction) ([]float32, map[domain.ContentKey]bool, bool) {
	known := make(map[domain.ContentKey]bool, len(interactions))
	var profile []float32
	for _, interaction := range interactions {
		known[interaction.ContentKey] = true
		vector, found := sceneVectors[interaction.ContentKey]
		if !found || interaction.Weight <= 0 {
			continue
		}
		if profile == nil {
			profile = make([]float32, len(vector))
		}
		if len(vector) != len(profile) {
			continue
		}
		for index, value := range vector {
			profile[index] += value * float32(interaction.Weight)
		}
	}
	if len(profile) == 0 {
		return nil, known, false
	}

	var squaredLength float64
	for _, value := range profile {
		squaredLength += float64(value * value)
	}
	length := math.Sqrt(squaredLength)
	if length == 0 {
		return nil, known, false
	}
	for index := range profile {
		profile[index] /= float32(length)
	}
	return profile, known, true
}
