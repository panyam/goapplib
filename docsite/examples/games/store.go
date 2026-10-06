// Package games is the app the views and routing guides walk through: a list of games with
// pagination and search, a page per game, a "new game" page behind a login, and a DELETE handler,
// grouped under /games. games_test.go drives it over HTTP, built both with Register and with
// MuxBuilder.
package games

import (
	"slices"
	"strings"
	"sync"
)

// Game is one game in the store.
type Game struct {
	ID   string
	Name string
}

// Store is an in-memory list of games, standing in for a real service client.
type Store struct {
	mu    sync.Mutex
	games []Game
}

// NewStore holds games, in order.
func NewStore(games ...Game) *Store { return &Store{games: games} }

// List returns the games whose names contain query, from offset, at most limit, and how many
// matched in all.
func (s *Store) List(query string, offset, limit int) ([]Game, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var matched []Game
	for _, g := range s.games {
		if strings.Contains(strings.ToLower(g.Name), strings.ToLower(query)) {
			matched = append(matched, g)
		}
	}
	end := min(offset+limit, len(matched))
	if offset >= end {
		return nil, len(matched)
	}
	return matched[offset:end], len(matched)
}

// Get returns the game with id, if there is one.
func (s *Store) Get(id string) (Game, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.games, func(g Game) bool { return g.ID == id })
	if i < 0 {
		return Game{}, false
	}
	return s.games[i], true
}

// Delete removes the game with id, reporting whether it was there.
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.games)
	s.games = slices.DeleteFunc(s.games, func(g Game) bool { return g.ID == id })
	return len(s.games) < n
}
