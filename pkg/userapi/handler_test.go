package userapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// seed creates a MemStore with one user and returns both.
func seed() (*MemStore, User) {
	st := NewMemStore()
	u := User{
		ID:        "usr-1",
		Name:      "Alice",
		Email:     "alice@example.com",
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	_ = st.Create(u)
	return st, u
}

// do sends a request to the mux and returns the response recorder.
func do(mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

// errBody extracts the "error" field from a JSON error response.
func errBody(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var e map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return e["error"]
}

func TestHealthz(t *testing.T) {
	mux := NewMux(NewMemStore())
	rr := do(mux, "GET", "/healthz", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body["ok"] {
		t.Fatal("expected ok=true")
	}
}

func TestCreateUser(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		want   int
		errMsg string // substring expected in error field; empty = no error check
	}{
		{
			name: "valid",
			body: `{"name":"Bob","email":"bob@example.com"}`,
			want: http.StatusCreated,
		},
		{
			name:   "missing name",
			body:   `{"name":"","email":"bob@example.com"}`,
			want:   http.StatusUnprocessableEntity,
			errMsg: "name is required",
		},
		{
			name:   "missing email",
			body:   `{"name":"Bob","email":""}`,
			want:   http.StatusUnprocessableEntity,
			errMsg: "email is required",
		},
		{
			name:   "invalid email no domain",
			body:   `{"name":"Bob","email":"bob@"}`,
			want:   http.StatusUnprocessableEntity,
			errMsg: "invalid email",
		},
		{
			name:   "invalid email no at",
			body:   `{"name":"Bob","email":"bobexample.com"}`,
			want:   http.StatusUnprocessableEntity,
			errMsg: "invalid email",
		},
		{
			name:   "invalid email no dot in domain",
			body:   `{"name":"Bob","email":"bob@example"}`,
			want:   http.StatusUnprocessableEntity,
			errMsg: "invalid email",
		},
		{
			name:   "invalid JSON",
			body:   `{bad`,
			want:   http.StatusBadRequest,
			errMsg: "invalid JSON",
		},
		{
			name:   "duplicate email",
			body:   `{"name":"Alice2","email":"alice@example.com"}`,
			want:   http.StatusConflict,
			errMsg: "email already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, _ := seed() // pre-seeded with alice@example.com
			mux := NewMux(st)
			rr := do(mux, "POST", "/api/v1/users", tt.body)

			if rr.Code != tt.want {
				t.Fatalf("status: got %d, want %d; body: %s", rr.Code, tt.want, rr.Body.String())
			}
			if tt.errMsg != "" {
				got := errBody(t, rr)
				if !strings.Contains(got, tt.errMsg) {
					t.Fatalf("error: got %q, want substring %q", got, tt.errMsg)
				}
			}
			if tt.want == http.StatusCreated {
				var u User
				if err := json.Unmarshal(rr.Body.Bytes(), &u); err != nil {
					t.Fatalf("decode user: %v", err)
				}
				if u.ID == "" {
					t.Fatal("expected non-empty ID")
				}
				if u.Name != "Bob" {
					t.Fatalf("name: got %q, want Bob", u.Name)
				}
				if u.Email != "bob@example.com" {
					t.Fatalf("email: got %q, want bob@example.com", u.Email)
				}
				if u.CreatedAt.IsZero() || u.UpdatedAt.IsZero() {
					t.Fatal("expected non-zero timestamps")
				}
			}
		})
	}
}

func TestListUsers(t *testing.T) {
	tests := []struct {
		name      string
		seedFunc  func() *MemStore
		wantCount int
	}{
		{
			name:      "empty store",
			seedFunc:  NewMemStore,
			wantCount: 0,
		},
		{
			name: "one user",
			seedFunc: func() *MemStore {
				st, _ := seed()
				return st
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := NewMux(tt.seedFunc())
			rr := do(mux, "GET", "/api/v1/users", "")

			if rr.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200", rr.Code)
			}
			var users []User
			if err := json.Unmarshal(rr.Body.Bytes(), &users); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(users) != tt.wantCount {
				t.Fatalf("count: got %d, want %d", len(users), tt.wantCount)
			}
		})
	}
}

func TestGetUser(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		want   int
		errMsg string
	}{
		{
			name: "found",
			id:   "usr-1",
			want: http.StatusOK,
		},
		{
			name:   "not found",
			id:     "usr-999",
			want:   http.StatusNotFound,
			errMsg: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, _ := seed()
			mux := NewMux(st)
			rr := do(mux, "GET", "/api/v1/users/"+tt.id, "")

			if rr.Code != tt.want {
				t.Fatalf("status: got %d, want %d", rr.Code, tt.want)
			}
			if tt.errMsg != "" {
				got := errBody(t, rr)
				if !strings.Contains(got, tt.errMsg) {
					t.Fatalf("error: got %q, want substring %q", got, tt.errMsg)
				}
			}
			if tt.want == http.StatusOK {
				var u User
				if err := json.Unmarshal(rr.Body.Bytes(), &u); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if u.ID != "usr-1" || u.Name != "Alice" {
					t.Fatalf("user: got %+v", u)
				}
			}
		})
	}
}

func TestUpdateUser(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		body   string
		want   int
		errMsg string
	}{
		{
			name: "valid update",
			id:   "usr-1",
			body: `{"name":"Alice Updated","email":"alice-new@example.com"}`,
			want: http.StatusOK,
		},
		{
			name:   "not found",
			id:     "usr-999",
			body:   `{"name":"X","email":"x@example.com"}`,
			want:   http.StatusNotFound,
			errMsg: "not found",
		},
		{
			name:   "missing name",
			id:     "usr-1",
			body:   `{"name":"","email":"a@b.com"}`,
			want:   http.StatusBadRequest,
			errMsg: "name is required",
		},
		{
			name:   "invalid email",
			id:     "usr-1",
			body:   `{"name":"Alice","email":"bad"}`,
			want:   http.StatusBadRequest,
			errMsg: "invalid email",
		},
		{
			name:   "invalid JSON",
			id:     "usr-1",
			body:   `{broken`,
			want:   http.StatusBadRequest,
			errMsg: "invalid JSON",
		},
		{
			name:   "email conflict",
			id:     "usr-1",
			body:   `{"name":"Alice","email":"other@example.com"}`,
			want:   http.StatusConflict,
			errMsg: "email already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, _ := seed()
			// For the conflict test, add a second user to conflict with.
			if tt.name == "email conflict" {
				_ = st.Create(User{
					ID: "usr-2", Name: "Other", Email: "other@example.com",
					CreatedAt: time.Now(), UpdatedAt: time.Now(),
				})
			}
			mux := NewMux(st)
			rr := do(mux, "PUT", "/api/v1/users/"+tt.id, tt.body)

			if rr.Code != tt.want {
				t.Fatalf("status: got %d, want %d; body: %s", rr.Code, tt.want, rr.Body.String())
			}
			if tt.errMsg != "" {
				got := errBody(t, rr)
				if !strings.Contains(got, tt.errMsg) {
					t.Fatalf("error: got %q, want substring %q", got, tt.errMsg)
				}
			}
			if tt.want == http.StatusOK {
				var u User
				if err := json.Unmarshal(rr.Body.Bytes(), &u); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if u.Name != "Alice Updated" {
					t.Fatalf("name: got %q, want Alice Updated", u.Name)
				}
				if u.ID != "usr-1" {
					t.Fatal("ID must not change on update")
				}
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		want   int
		errMsg string
	}{
		{
			name: "found",
			id:   "usr-1",
			want: http.StatusNoContent,
		},
		{
			name:   "not found",
			id:     "usr-999",
			want:   http.StatusNotFound,
			errMsg: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, _ := seed()
			mux := NewMux(st)
			rr := do(mux, "DELETE", "/api/v1/users/"+tt.id, "")

			if rr.Code != tt.want {
				t.Fatalf("status: got %d, want %d", rr.Code, tt.want)
			}
			if tt.errMsg != "" {
				got := errBody(t, rr)
				if !strings.Contains(got, tt.errMsg) {
					t.Fatalf("error: got %q, want substring %q", got, tt.errMsg)
				}
			}
			// Verify deletion: GET should 404.
			if tt.want == http.StatusNoContent {
				rr2 := do(mux, "GET", "/api/v1/users/"+tt.id, "")
				if rr2.Code != http.StatusNotFound {
					t.Fatalf("after delete: expected 404, got %d", rr2.Code)
				}
			}
		})
	}
}
