package userapi

import "sync"

// MemStore is an in-memory Store backed by a map. Thread-safe via RWMutex.
type MemStore struct {
	mu    sync.RWMutex
	users map[string]User
}

// NewMemStore returns an empty in-memory user store.
func NewMemStore() *MemStore {
	return &MemStore{users: make(map[string]User)}
}

func (s *MemStore) Create(u User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.users {
		if existing.Email == u.Email {
			return ErrConflict
		}
	}
	s.users[u.ID] = u
	return nil
}

func (s *MemStore) Get(id string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (s *MemStore) List() ([]User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	return out, nil
}

func (s *MemStore) Update(u User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[u.ID]; !ok {
		return ErrNotFound
	}
	// Check email uniqueness against other users.
	for _, existing := range s.users {
		if existing.Email == u.Email && existing.ID != u.ID {
			return ErrConflict
		}
	}
	s.users[u.ID] = u
	return nil
}

func (s *MemStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[id]; !ok {
		return ErrNotFound
	}
	delete(s.users, id)
	return nil
}
