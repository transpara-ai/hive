package knowledge

import (
	"fmt"
	"sort"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// ReplayFromStore reconstructs knowledge store state by reading all knowledge
// events from the event chain. Events are fetched via 3 separate ByType calls
// (one per knowledge event type), sorted chronologically, then replayed.
func ReplayFromStore(s store.Store, ks KnowledgeStore) error {
	knowledgeTypes := []types.EventType{
		event.EventTypeKnowledgeInsightRecorded,
		event.EventTypeKnowledgeInsightSuperseded,
		event.EventTypeKnowledgeInsightExpired,
	}

	var allEvents []event.Event

	for _, et := range knowledgeTypes {
		events, err := fetchAllByType(s, et)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", et.Value(), err)
		}
		allEvents = append(allEvents, events...)
	}

	// ByType returns reverse-chrono. Sort ascending (oldest first) for replay.
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Timestamp().Value().Before(allEvents[j].Timestamp().Value())
	})

	for _, ev := range allEvents {
		switch c := ev.Content().(type) {
		case event.KnowledgeInsightContent:
			insight := ConvertFromEventContent(c, ev.Timestamp().Value())
			if err := ks.Record(insight); err != nil {
				return fmt.Errorf("record insight %s: %w", c.InsightID, err)
			}
			// Handle inline supersession.
			if c.SupersedesID != "" {
				if err := ks.Supersede(c.SupersedesID, c.InsightID); err != nil {
					return fmt.Errorf("supersede %s: %w", c.SupersedesID, err)
				}
			}

		case event.KnowledgeSupersessionContent:
			if err := ks.Supersede(c.OldInsightID, c.NewInsightID); err != nil {
				return fmt.Errorf("supersede %s→%s: %w", c.OldInsightID, c.NewInsightID, err)
			}

		case event.KnowledgeExpirationContent:
			if err := ks.Expire(c.InsightID); err != nil {
				return fmt.Errorf("expire %s: %w", c.InsightID, err)
			}
		}
	}

	return nil
}

// ConvertFromEventContent maps an event content struct to an in-memory
// KnowledgeInsight. TTL (in hours) is converted to an absolute ExpiresAt.
func ConvertFromEventContent(c event.KnowledgeInsightContent, recordedAt time.Time) KnowledgeInsight {
	var expiresAt time.Time
	if c.TTL > 0 {
		expiresAt = recordedAt.Add(time.Duration(c.TTL) * time.Hour)
	}

	return KnowledgeInsight{
		InsightID:     c.InsightID,
		Domain:        c.Domain,
		Summary:       c.Summary,
		RelevantRoles: c.RelevantRoles,
		Confidence:    c.Confidence,
		EvidenceCount: c.EvidenceCount,
		Source:        c.Source,
		RecordedAt:    recordedAt,
		ExpiresAt:     expiresAt,
		Active:        true,
	}
}

// fetchAllByType pages through all events of a given type.
func fetchAllByType(s store.Store, et types.EventType) ([]event.Event, error) {
	const pageSize = 1000
	var all []event.Event
	cursor := types.None[types.Cursor]()

	for {
		page, err := s.ByType(et, pageSize, cursor)
		if err != nil {
			return nil, err
		}
		items := page.Items()
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}

	return all, nil
}
