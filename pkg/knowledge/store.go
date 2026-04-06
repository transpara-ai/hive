package knowledge

import (
	"context"
	"sort"
	"sync"
	"time"
)

// KnowledgeStore manages the lifecycle and retrieval of knowledge insights.
type KnowledgeStore interface {
	Record(insight KnowledgeInsight) error
	Supersede(oldID, newID string) error
	Expire(insightID string) error
	Query(filter KnowledgeFilter, maxResults int) []KnowledgeInsight
	ActiveCount() int
	PruneExpired() int
}

type store struct {
	mu       sync.RWMutex
	insights map[string]*KnowledgeInsight
}

// NewStore creates an empty in-memory KnowledgeStore.
func NewStore() KnowledgeStore {
	return &store{
		insights: make(map[string]*KnowledgeInsight),
	}
}

func (s *store) Record(insight KnowledgeInsight) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	insight.Active = true
	s.insights[insight.InsightID] = &insight

	s.enforceLimit()
	return nil
}

func (s *store) Supersede(oldID, newID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.insights[oldID]; ok {
		old.Active = false
	}
	// Don't error if old not found — may have already expired.
	return nil
}

func (s *store) Expire(insightID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ins, ok := s.insights[insightID]; ok {
		ins.Active = false
	}
	return nil
}

func (s *store) Query(filter KnowledgeFilter, maxResults int) []KnowledgeInsight {
	s.mu.RLock()
	defer s.mu.RUnlock()

	excludeSet := make(map[string]struct{}, len(filter.ExcludeIDs))
	for _, id := range filter.ExcludeIDs {
		excludeSet[id] = struct{}{}
	}

	domainSet := make(map[string]struct{}, len(filter.Domains))
	for _, d := range filter.Domains {
		domainSet[d] = struct{}{}
	}

	type scored struct {
		insight KnowledgeInsight
		score   float64
	}
	var results []scored

	for _, ins := range s.insights {
		if !ins.Active {
			continue
		}
		if _, excluded := excludeSet[ins.InsightID]; excluded {
			continue
		}
		if !roleMatch(ins.RelevantRoles, filter.Role) {
			continue
		}
		if len(domainSet) > 0 {
			if _, ok := domainSet[ins.Domain]; !ok {
				continue
			}
		}
		if ins.Confidence < filter.MinConfidence {
			continue
		}
		if filter.MaxAge > 0 && time.Since(ins.RecordedAt) > filter.MaxAge {
			continue
		}

		results = append(results, scored{
			insight: *ins,
			score:   relevanceScore(*ins, filter),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}

	out := make([]KnowledgeInsight, len(results))
	for i, r := range results {
		out[i] = r.insight
	}
	return out
}

func (s *store) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, ins := range s.insights {
		if ins.Active {
			count++
		}
	}
	return count
}

func (s *store) PruneExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	pruned := 0
	for _, ins := range s.insights {
		if ins.Active && !ins.ExpiresAt.IsZero() && now.After(ins.ExpiresAt) {
			ins.Active = false
			pruned++
		}
	}
	return pruned
}

// RunPruner starts a background goroutine that prunes expired insights on
// the given interval. It exits when ctx is cancelled.
func RunPruner(ctx context.Context, s KnowledgeStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.PruneExpired()
		}
	}
}

// enforceLimit deactivates the lowest-confidence active insights when the
// active count exceeds MaxActiveInsights. Must be called with mu held.
func (s *store) enforceLimit() {
	active := make([]*KnowledgeInsight, 0)
	for _, ins := range s.insights {
		if ins.Active {
			active = append(active, ins)
		}
	}

	if len(active) <= MaxActiveInsights {
		return
	}

	// Sort ascending by confidence so we deactivate lowest first.
	sort.Slice(active, func(i, j int) bool {
		return active[i].Confidence < active[j].Confidence
	})

	excess := len(active) - MaxActiveInsights
	for i := 0; i < excess; i++ {
		active[i].Active = false
	}
}

// roleMatch returns true if the insight is relevant for the given role.
// An insight with empty RelevantRoles is universal (relevant to all roles).
func roleMatch(relevantRoles []string, role string) bool {
	if len(relevantRoles) == 0 {
		return true // universal insight
	}
	if role == "" {
		return true // no role filter
	}
	for _, r := range relevantRoles {
		if r == role {
			return true
		}
	}
	return false
}

// relevanceScore computes a 0.0–1.0 relevance score for ranking query results.
func relevanceScore(ins KnowledgeInsight, filter KnowledgeFilter) float64 {
	score := 0.0

	// Role match: 0.4 if explicitly listed, 0.2 if universal.
	if len(ins.RelevantRoles) == 0 {
		score += 0.2
	} else {
		for _, r := range ins.RelevantRoles {
			if r == filter.Role {
				score += 0.4
				break
			}
		}
	}

	// Confidence contributes up to 0.3.
	score += ins.Confidence * 0.3

	// Recency.
	age := time.Since(ins.RecordedAt)
	switch {
	case age < 1*time.Hour:
		score += 0.2
	case age < 6*time.Hour:
		score += 0.15
	case age < 24*time.Hour:
		score += 0.1
	default:
		score += 0.05
	}

	// Evidence count.
	switch {
	case ins.EvidenceCount >= 10:
		score += 0.1
	case ins.EvidenceCount >= 5:
		score += 0.07
	default:
		score += 0.03
	}

	return score
}
