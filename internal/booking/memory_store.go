package booking

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore keeps bookings in process memory. Handy for tests and local
// runs without Redis; holds expire lazily when they are read.
type MemoryStore struct {
	mu       sync.Mutex
	holdTTL  time.Duration
	bySeat   map[string]Booking // movieID:seatID -> booking
	sessions map[string]string  // sessionID -> seat key
}

func NewMemoryStore(holdTTL time.Duration) *MemoryStore {
	return &MemoryStore{
		holdTTL:  holdTTL,
		bySeat:   map[string]Booking{},
		sessions: map[string]string{},
	}
}

func seatKey(movieID, seatID string) string {
	return movieID + ":" + seatID
}

func (m *MemoryStore) expire(key string) {
	b, ok := m.bySeat[key]
	if ok && b.Status == StatusHeld && time.Now().After(b.ExpiresAt) {
		delete(m.bySeat, key)
		delete(m.sessions, b.ID)
	}
}

func (m *MemoryStore) Hold(_ context.Context, b Booking) (Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := seatKey(b.MovieID, b.SeatID)
	m.expire(key)
	if _, taken := m.bySeat[key]; taken {
		return Booking{}, ErrSeatTaken
	}

	b.ID = uuid.NewString()
	b.Status = StatusHeld
	b.ExpiresAt = time.Now().Add(m.holdTTL)

	m.bySeat[key] = b
	m.sessions[b.ID] = key
	return b, nil
}

func (m *MemoryStore) List(_ context.Context, movieID string) ([]Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Booking
	for key, b := range m.bySeat {
		m.expire(key)
		if _, ok := m.bySeat[key]; ok && b.MovieID == movieID {
			out = append(out, b)
		}
	}
	return out, nil
}

// owned returns the seat key and booking for a session the user owns.
func (m *MemoryStore) owned(sessionID, userID string) (string, Booking, error) {
	key, ok := m.sessions[sessionID]
	if !ok {
		return "", Booking{}, ErrSessionNotFound
	}
	m.expire(key)
	b, ok := m.bySeat[key]
	if !ok {
		return "", Booking{}, ErrSessionNotFound
	}
	if b.UserID != userID {
		return "", Booking{}, ErrNotSessionOwner
	}
	return key, b, nil
}

func (m *MemoryStore) Confirm(_ context.Context, sessionID, userID string) (Booking, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, b, err := m.owned(sessionID, userID)
	if err != nil {
		return Booking{}, err
	}
	b.Status = StatusConfirmed
	b.ExpiresAt = time.Time{}
	m.bySeat[key] = b
	return b, nil
}

func (m *MemoryStore) Release(_ context.Context, sessionID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, _, err := m.owned(sessionID, userID)
	if err != nil {
		return err
	}
	delete(m.bySeat, key)
	delete(m.sessions, sessionID)
	return nil
}
