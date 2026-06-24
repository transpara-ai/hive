package hive

import (
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func eventsByTypePaginated(s store.Store, eventType types.EventType, pageSize int) ([]event.Event, error) {
	if s == nil {
		return nil, fmt.Errorf("store is required")
	}
	if pageSize <= 0 {
		pageSize = defaultOperatorProjectionLimit
	}
	cursor := types.None[types.Cursor]()
	var out []event.Event
	for {
		page, err := s.ByType(eventType, pageSize, cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items()...)
		if !page.HasMore() {
			return out, nil
		}
		cursor = page.Cursor()
	}
}
