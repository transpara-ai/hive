package authority

import (
	"testing"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

func testEventID(t *testing.T) types.EventID {
	t.Helper()
	id, err := types.NewEventIDFromNew()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testActorID(t *testing.T) types.ActorID {
	t.Helper()
	// ActorIDs are derived from public keys, use a valid format.
	id := types.MustActorID("actor_deadbeef0000000000000000000001")
	return id
}

func TestNotificationAutoApproves(t *testing.T) {
	gate := NewGate(nil)
	req := Request{
		ID:    testEventID(t),
		Actor: testActorID(t),
		Level: event.AuthorityLevelNotification,
	}
	res := gate.Check(req)
	if !res.Approved {
		t.Fatal("notification should auto-approve")
	}
}

func TestRequiredDeniedWithoutApprover(t *testing.T) {
	gate := NewGate(nil)
	req := Request{
		ID:    testEventID(t),
		Actor: testActorID(t),
		Level: event.AuthorityLevelRequired,
	}
	res := gate.Check(req)
	if res.Approved {
		t.Fatal("Required without approver should deny")
	}
}

func TestRequiredApprovedByHuman(t *testing.T) {
	gate := NewGate(func(req Request) (bool, string) {
		return true, "looks good"
	})
	req := Request{
		ID:            testEventID(t),
		Actor:         testActorID(t),
		Level:         event.AuthorityLevelRequired,
		Justification: "need a builder",
	}
	res := gate.Check(req)
	if !res.Approved {
		t.Fatal("human approved, should be approved")
	}
	if res.Reason != "looks good" {
		t.Errorf("reason = %q, want %q", res.Reason, "looks good")
	}
}

func TestRequiredDeniedByHuman(t *testing.T) {
	gate := NewGate(func(req Request) (bool, string) {
		return false, "not needed"
	})
	req := Request{
		ID:    testEventID(t),
		Actor: testActorID(t),
		Level: event.AuthorityLevelRequired,
	}
	res := gate.Check(req)
	if res.Approved {
		t.Fatal("human denied, should not be approved")
	}
}

func TestRecommendedAutoApprovesOnTimeout(t *testing.T) {
	// Use a blocking approver that never responds.
	gate := NewGate(func(req Request) (bool, string) {
		time.Sleep(5 * time.Second) // will be interrupted by timeout
		return false, "too slow"
	})
	gate.SetRecommendedTimeout(50 * time.Millisecond)

	req := Request{
		ID:    testEventID(t),
		Actor: testActorID(t),
		Level: event.AuthorityLevelRecommended,
	}
	res := gate.Check(req)
	if !res.Approved {
		t.Fatal("recommended should auto-approve on timeout")
	}
}

func TestRecommendedApprovedByHumanBeforeTimeout(t *testing.T) {
	gate := NewGate(func(req Request) (bool, string) {
		return true, "approved quickly"
	})
	gate.SetRecommendedTimeout(10 * time.Second)

	req := Request{
		ID:    testEventID(t),
		Actor: testActorID(t),
		Level: event.AuthorityLevelRecommended,
	}
	res := gate.Check(req)
	if !res.Approved {
		t.Fatal("should be approved by human")
	}
	if res.Reason != "approved quickly" {
		t.Errorf("reason = %q, want %q", res.Reason, "approved quickly")
	}
}

func TestRecommendedWithoutApprover(t *testing.T) {
	gate := NewGate(nil)
	req := Request{
		ID:    testEventID(t),
		Actor: testActorID(t),
		Level: event.AuthorityLevelRecommended,
	}
	res := gate.Check(req)
	if !res.Approved {
		t.Fatal("recommended without approver should auto-approve")
	}
}

func TestPendingRequests(t *testing.T) {
	ch := make(chan bool)
	gate := NewGate(func(req Request) (bool, string) {
		<-ch
		return true, "done"
	})

	go func() {
		gate.Check(Request{
			ID:    testEventID(t),
			Actor: testActorID(t),
			Level: event.AuthorityLevelRequired,
		})
	}()

	// Give the goroutine time to start.
	time.Sleep(50 * time.Millisecond)

	pending := gate.Pending()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending, got %d", len(pending))
	}

	ch <- true // unblock
	time.Sleep(50 * time.Millisecond)

	pending = gate.Pending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after resolution, got %d", len(pending))
	}
}

func TestUnknownLevelDenied(t *testing.T) {
	gate := NewGate(nil)
	req := Request{
		ID:    testEventID(t),
		Actor: testActorID(t),
		Level: event.AuthorityLevel("bogus"),
	}
	res := gate.Check(req)
	if res.Approved {
		t.Fatal("unknown level should deny")
	}
}
