package main

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/social"
	"github.com/transpara-ai/work"
)

func TestRegisterSharedEventTypesCoversExistingGraphEvents(t *testing.T) {
	registerSharedEventTypes()

	cases := []struct {
		name      string
		eventType string
		body      []byte
		want      any
	}{
		{
			name:      "hive run completed",
			eventType: "hive.run.completed",
			body:      []byte(`{"AgentCount":1,"DurationMs":25,"TotalCost":0.1}`),
			want:      hive.RunCompletedContent{},
		},
		{
			name:      "work task created",
			eventType: "work.task.created",
			body:      []byte(`{"Title":"Review social MVP","CreatedBy":"actor_test","Priority":"medium","Workspace":"journey-test"}`),
			want:      work.TaskCreatedContent{},
		},
		{
			name:      "social post created",
			eventType: "social.post.created",
			body:      []byte(`{"Author":"actor_test","Body":"hello","Tags":["review"],"Workspace":"journey-test"}`),
			want:      social.PostCreatedContent{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := event.UnmarshalContent(tc.eventType, tc.body)
			if err != nil {
				t.Fatalf("UnmarshalContent(%s): %v", tc.eventType, err)
			}
			if _, ok := got.(interface{ EventTypeName() string }); !ok {
				t.Fatalf("got %T, want event content", got)
			}
			switch tc.want.(type) {
			case hive.RunCompletedContent:
				if _, ok := got.(hive.RunCompletedContent); !ok {
					t.Fatalf("got %T, want hive.RunCompletedContent", got)
				}
			case work.TaskCreatedContent:
				if _, ok := got.(work.TaskCreatedContent); !ok {
					t.Fatalf("got %T, want work.TaskCreatedContent", got)
				}
			case social.PostCreatedContent:
				if _, ok := got.(social.PostCreatedContent); !ok {
					t.Fatalf("got %T, want social.PostCreatedContent", got)
				}
			}
		})
	}
}
