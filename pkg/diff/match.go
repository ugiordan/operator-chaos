package diff

import (
	"math"
	"sort"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

const similarityThreshold = 0.6

type componentMatch struct {
	source  model.ComponentModel
	target  model.ComponentModel
	renamed bool
}

type resourceMatch struct {
	source  model.ManagedResource
	target  model.ManagedResource
	renamed bool
}

type candidate struct {
	si, ti int
	score  float64
}

// matchComponents pairs source and target components using two passes:
// first exact name match, then fuzzy matching on unmatched components
// based on structural similarity.
func matchComponents(_ string, source, target []model.ComponentModel) (matched []componentMatch, added []model.ComponentModel, removed []model.ComponentModel) {
	usedSource := make(map[int]bool)
	usedTarget := make(map[int]bool)

	// Pass 1: exact name match
	for si, s := range source {
		for ti, tgt := range target {
			if usedTarget[ti] {
				continue
			}
			if s.Name == tgt.Name {
				matched = append(matched, componentMatch{source: s, target: tgt, renamed: false})
				usedSource[si] = true
				usedTarget[ti] = true
				break
			}
		}
	}

	// Pass 2: fuzzy match remaining by structural similarity
	var candidates []candidate
	for si, s := range source {
		if usedSource[si] {
			continue
		}
		for ti, tgt := range target {
			if usedTarget[ti] {
				continue
			}
			score := componentSimilarity(s, tgt)
			if score >= similarityThreshold {
				candidates = append(candidates, candidate{si: si, ti: ti, score: score})
			}
		}
	}

	sortCandidatesByScore(candidates)

	for _, c := range candidates {
		if usedSource[c.si] || usedTarget[c.ti] {
			continue
		}
		matched = append(matched, componentMatch{
			source:  source[c.si],
			target:  target[c.ti],
			renamed: true,
		})
		usedSource[c.si] = true
		usedTarget[c.ti] = true
	}

	// Collect unmatched
	for si, s := range source {
		if !usedSource[si] {
			removed = append(removed, s)
		}
	}
	for ti, tgt := range target {
		if !usedTarget[ti] {
			added = append(added, tgt)
		}
	}

	return matched, added, removed
}

// componentSimilarity computes a weighted similarity score between two components.
// Weights: 0.4 Kind overlap + 0.3 label key overlap + 0.2 controller match + 0.1 count similarity
func componentSimilarity(a, b model.ComponentModel) float64 {
	kindOverlap := setOverlap(resourceKinds(a.ManagedResources), resourceKinds(b.ManagedResources))

	labelOverlap := setOverlap(allLabelKeys(a.ManagedResources), allLabelKeys(b.ManagedResources))

	controllerMatch := 0.0
	if a.Controller == b.Controller {
		controllerMatch = 1.0
	}

	countA := float64(len(a.ManagedResources))
	countB := float64(len(b.ManagedResources))
	countSim := 0.0
	maxCount := math.Max(countA, countB)
	if maxCount > 0 {
		countSim = 1.0 - math.Abs(countA-countB)/maxCount
	}

	return 0.4*kindOverlap + 0.3*labelOverlap + 0.2*controllerMatch + 0.1*countSim
}

// matchResources pairs source and target resources using two passes:
// first exact Kind+Name match, then fuzzy matching by Kind + label overlap.
func matchResources(source, target []model.ManagedResource) (matched []resourceMatch, added []model.ManagedResource, removed []model.ManagedResource) {
	usedSource := make(map[int]bool)
	usedTarget := make(map[int]bool)

	// Pass 1: exact Kind+Name match
	for si, s := range source {
		for ti, tgt := range target {
			if usedTarget[ti] {
				continue
			}
			if s.Kind == tgt.Kind && s.Name == tgt.Name {
				matched = append(matched, resourceMatch{source: s, target: tgt, renamed: false})
				usedSource[si] = true
				usedTarget[ti] = true
				break
			}
		}
	}

	// Pass 2: fuzzy match remaining by Kind + label overlap
	var candidates []candidate
	for si, s := range source {
		if usedSource[si] {
			continue
		}
		for ti, tgt := range target {
			if usedTarget[ti] {
				continue
			}
			if s.Kind != tgt.Kind {
				continue
			}
			overlap := setOverlap(labelKeySet(s.Labels), labelKeySet(tgt.Labels))
			if overlap >= similarityThreshold {
				candidates = append(candidates, candidate{si: si, ti: ti, score: overlap})
			}
		}
	}

	sortCandidatesByScore(candidates)

	for _, c := range candidates {
		if usedSource[c.si] || usedTarget[c.ti] {
			continue
		}
		matched = append(matched, resourceMatch{
			source:  source[c.si],
			target:  target[c.ti],
			renamed: true,
		})
		usedSource[c.si] = true
		usedTarget[c.ti] = true
	}

	// Collect unmatched
	for si, s := range source {
		if !usedSource[si] {
			removed = append(removed, s)
		}
	}
	for ti, tgt := range target {
		if !usedTarget[ti] {
			added = append(added, tgt)
		}
	}

	return matched, added, removed
}

// resourceKinds collects the distinct Kind values from a slice of resources.
func resourceKinds(resources []model.ManagedResource) map[string]bool {
	kinds := make(map[string]bool)
	for _, r := range resources {
		kinds[r.Kind] = true
	}
	return kinds
}

// allLabelKeys collects every distinct label key across all resources.
func allLabelKeys(resources []model.ManagedResource) map[string]bool {
	keys := make(map[string]bool)
	for _, r := range resources {
		for k := range r.Labels {
			keys[k] = true
		}
	}
	return keys
}

// labelKeySet returns the set of label keys for a single resource's labels.
func labelKeySet(labels map[string]string) map[string]bool {
	keys := make(map[string]bool)
	for k := range labels {
		keys[k] = true
	}
	return keys
}

// setOverlap computes the Jaccard similarity: |intersection| / |union|.
// Returns 0 if both sets are empty.
func setOverlap(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}

	union := len(a)
	for k := range b {
		if !a[k] {
			union++
		}
	}

	return float64(intersection) / float64(union)
}

// sortCandidatesByScore sorts candidates in descending order by score.
func sortCandidatesByScore(candidates []candidate) {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
}
