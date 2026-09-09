package civilization

import (
	"context"
	"net/http"
	"regexp"
	"strings"
)

type humanIdentityKey struct{}

func humanActor(ctx context.Context) string {
	actor, _ := ctx.Value(humanIdentityKey{}).(string)
	return actor
}

var humanActorPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

// The bearer-authenticated Site is the assertion issuer. Browser-provided
// identity and role headers are never forwarded by Site.
func humanActionAllowed(actor, role, method, path string) bool {
	if !humanActorPattern.MatchString(actor) || actor == "anonymous" || (role != "viewer" && role != "operator" && role != "reviewer") {
		return false
	}
	if method == http.MethodGet || method == http.MethodHead {
		return true
	}
	if method != http.MethodPost || role == "viewer" {
		return false
	}
	if path == "/api/civilization/v1/intake" {
		return true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/api/civilization/v1/work/"), "/")
	if !strings.HasPrefix(path, "/api/civilization/v1/work/") || len(parts) < 2 || parts[0] == "" {
		return false
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "run", "human-owner":
			return true
		case "confirm", "result-review":
			return role == "reviewer"
		}
	}
	return len(parts) == 4 && parts[1] == "interventions" && parts[2] != "" && parts[3] == "resolve"
}
