package utils

import (
	"sort"
	"sync"
	"time"
)

// TimeHistogram représente un histogramme pour mesurer des durées
type TimeHistogram struct {
	mu      sync.RWMutex
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

// NewTimeHistogram crée un nouvel histogramme avec les buckets spécifiés
func NewTimeHistogram(buckets []float64) *TimeHistogram {
	// Trier les buckets pour s'assurer qu'ils sont dans l'ordre croissant
	sortedBuckets := make([]float64, len(buckets))
	copy(sortedBuckets, buckets)
	sort.Float64s(sortedBuckets)

	return &TimeHistogram{
		buckets: sortedBuckets,
		counts:  make([]uint64, len(sortedBuckets)+1), // +1 pour le bucket +Inf
	}
}

// Observe enregistre une nouvelle valeur dans l'histogramme
func (h *TimeHistogram) Observe(duration time.Duration) {
	seconds := duration.Seconds()

	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += seconds
	h.count++

	// Trouver le bucket approprié
	for i, upperBound := range h.buckets {
		if seconds <= upperBound {
			h.counts[i]++
			return
		}
	}
	// Si on arrive ici, la valeur va dans le dernier bucket (+Inf)
	h.counts[len(h.buckets)]++
}

// GetMetrics retourne les métriques de l'histogramme
func (h *TimeHistogram) GetMetrics() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	bucketValues := make(map[float64]uint64)
	for i, upperBound := range h.buckets {
		bucketValues[upperBound] = h.counts[i]
	}
	bucketValues[float64(0)] = h.counts[len(h.buckets)] // Le bucket +Inf

	return map[string]interface{}{
		"count":   h.count,
		"sum":     h.sum,
		"avg":     h.sum / float64(h.count),
		"buckets": bucketValues,
	}
}

// Reset réinitialise l'histogramme
func (h *TimeHistogram) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum = 0
	h.count = 0
	for i := range h.counts {
		h.counts[i] = 0
	}
}

// GetBuckets retourne les limites des buckets
func (h *TimeHistogram) GetBuckets() []float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	buckets := make([]float64, len(h.buckets))
	copy(buckets, h.buckets)
	return buckets
}

// GetCount retourne le nombre total d'observations
func (h *TimeHistogram) GetCount() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.count
}

// GetSum retourne la somme de toutes les observations
func (h *TimeHistogram) GetSum() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sum
}

// GetAverage retourne la moyenne des observations
func (h *TimeHistogram) GetAverage() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.count == 0 {
		return 0
	}
	return h.sum / float64(h.count)
}
