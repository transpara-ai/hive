package userapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// NewMux returns a ServeMux with all user-api routes registered.
// Handlers are closures that capture the store — no globals.
func NewMux(st Store) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("POST /api/v1/users", createHandler(st))
	mux.HandleFunc("GET /api/v1/users", listHandler(st))
	mux.HandleFunc("GET /api/v1/users/{id}", getHandler(st))
	mux.HandleFunc("PUT /api/v1/users/{id}", updateHandler(st))
	mux.HandleFunc("DELETE /api/v1/users/{id}", deleteHandler(st))

	return mux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func createHandler(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := req.Validate(); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		now := time.Now().UTC()
		u := User{
			ID:        NewID(),
			Name:      req.Name,
			Email:     req.Email,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := st.Create(u); err != nil {
			if errors.Is(err, ErrConflict) {
				writeErr(w, http.StatusConflict, err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("create: %v", err))
			return
		}
		writeJSON(w, http.StatusCreated, u)
	}
}

func listHandler(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		users, err := st.List()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("list: %v", err))
			return
		}
		writeJSON(w, http.StatusOK, users)
	}
}

func getHandler(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		u, err := st.Get(id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, http.StatusNotFound, fmt.Sprintf("user %q not found", id))
				return
			}
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("get: %v", err))
			return
		}
		writeJSON(w, http.StatusOK, u)
	}
}

func updateHandler(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		existing, err := st.Get(id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, http.StatusNotFound, fmt.Sprintf("user %q not found", id))
				return
			}
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("get: %v", err))
			return
		}

		var req UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := req.Validate(); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}

		existing.Name = req.Name
		existing.Email = req.Email
		existing.UpdatedAt = time.Now().UTC()

		if err := st.Update(existing); err != nil {
			if errors.Is(err, ErrConflict) {
				writeErr(w, http.StatusConflict, err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("update: %v", err))
			return
		}
		writeJSON(w, http.StatusOK, existing)
	}
}

func deleteHandler(st Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := st.Delete(id); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, http.StatusNotFound, fmt.Sprintf("user %q not found", id))
				return
			}
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("delete: %v", err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
