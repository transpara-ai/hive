package main

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/hive/pkg/social"
)

func TestRegisterOpsAPIEventTypesHandlesSharedStoreEvents(t *testing.T) {
	registerOpsAPIEventTypes()
	t.Cleanup(func() { event.SetFallbackUnmarshaler(nil) })

	got, err := event.UnmarshalContent(social.EventTypePostCreated.Value(), []byte(`{"Author":"actor_00000000000000000000000000000077","Body":"hello"}`))
	if err != nil {
		t.Fatalf("unmarshal social post: %v", err)
	}
	if _, ok := got.(social.PostCreatedContent); !ok {
		t.Fatalf("social post content type = %T, want social.PostCreatedContent", got)
	}

	raw, err := event.UnmarshalContent("foreign.event.type", []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("unmarshal unknown shared-store event: %v", err)
	}
	if _, ok := raw.(event.RawContent); !ok {
		t.Fatalf("unknown event content type = %T, want event.RawContent", raw)
	}

	if _, err := event.UnmarshalContent(social.EventTypePostCreated.Value(), []byte(`{`)); err == nil {
		t.Fatal("malformed registered social event decoded successfully; fallback must only handle unknown event types")
	}
}
