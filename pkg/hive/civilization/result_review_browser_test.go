package civilization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

type reviewBrowserEffects struct{ *fakeEffects }

func (*reviewBrowserEffects) PublicationEnabled() bool { return false }
func (e *reviewBrowserEffects) PreparedArtifact(_ context.Context, workID string, bound tlcbridge.BoundRequest, _ Workspace, _ ProviderResult, digest string) (Artifact, error) {
	return Artifact{Repository: bound.Source.Repository, Branch: "civilization/" + workID, WorkspaceDigest: digest, Patch: "New file: result.txt\nDelivered test result\n"}, nil
}

// Opt-in cross-repository browser fixture uses the real HTTP and lifecycle
// engine with deterministic providers/effects. No deployed work is mutated.
func TestResultReviewBrowserFixture(t *testing.T) {
	output := os.Getenv("RESULT_REVIEW_BROWSER_FIXTURE_FILE")
	if output == "" {
		t.Skip("opt-in browser fixture")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine, _, effects := newTestEngine(t, "Routine", false)
	engine.effects = &reviewBrowserEffects{effects}
	ids := map[string]string{}
	for _, name := range []string{"approve", "reject", "enhance"} {
		item, err := engine.SubmitText(ctx, tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:anonymous:" + name, Repository: "transpara-ai/hive"}, "Review browser result "+name)
		if err != nil {
			t.Fatal(err)
		}
		item, err = engine.Run(ctx, item.WorkID)
		if err != nil || item.State != StatePrepared {
			t.Fatalf("fixture=%+v %v", item, err)
		}
		ids[name] = item.WorkID
	}
	api, err := NewHTTPHandler(HTTPConfig{Engine: engine, APIKey: strings.Repeat("k", 32)})
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.Handle("/", api)
	mux.HandleFunc("POST /__fixture/stop", func(w http.ResponseWriter, r *http.Request) {
		select {
		case stopped <- struct{}{}:
		default:
		}
		w.WriteHeader(204)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	raw, _ := json.Marshal(map[string]any{"base": server.URL, "work": ids})
	if err := os.WriteFile(output, raw, 0600); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-stopped:
			return
		case <-deadline.C:
			t.Fatal("browser fixture timed out")
		case <-ticker.C:
			items, err := engine.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range items {
				if item.State == StateRouting || item.State == StateQueued {
					if _, err = engine.Advance(ctx, item.WorkID); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}
