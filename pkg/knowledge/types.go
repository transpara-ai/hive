package knowledge

import "time"

// Domain constants for categorising insights.
const (
	DomainHealth       = "health"
	DomainBudget       = "budget"
	DomainQuality      = "quality"
	DomainArchitecture = "architecture"
	DomainProcess      = "process"
	DomainPerformance  = "performance"
	DomainPatterns     = "patterns"
)

// Source prefixes and identifiers.
const (
	SourceDistillerPrefix = "distiller:"
	SourceMemoryKeeper    = "memorykeeper"
	SourceOperator        = "operator"
)

// Cardinality limits.
const (
	MaxActiveInsights  = 100
	MaxPerDomain       = 20
	MaxPerSource       = 50
	MaxEnrichmentItems = 5
	MaxItemChars       = 300
	MaxBlockChars      = 1800
)

// KnowledgeInsight is the in-memory representation of a single piece of
// distilled knowledge that can be injected into agent context.
type KnowledgeInsight struct {
	InsightID     string
	Domain        string
	Summary       string
	RelevantRoles []string
	Confidence    float64
	EvidenceCount int
	Source        string
	RecordedAt    time.Time
	ExpiresAt     time.Time // zero value = never expires
	Active        bool
}

// KnowledgeFilter specifies query parameters for retrieving insights.
type KnowledgeFilter struct {
	Role          string        // filter by role relevance
	Domains       []string      // filter by domain (empty = all)
	MinConfidence float64       // default 0.3
	MaxAge        time.Duration // zero = no age filter
	ExcludeIDs    []string      // already-seen insight IDs
}
