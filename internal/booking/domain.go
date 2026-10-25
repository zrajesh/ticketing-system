package booking

import (
	"context"
	"errors"
	"time"
)

const (
	StatusHeld      = "held"
	StatusConfirmed = "confirmed"
)

var (
	ErrSeatTaken       = errors.New("seat is already taken")
	ErrSessionNotFound = errors.New("session not found or expired")
	ErrNotSessionOwner = errors.New("session belongs to another user")
)

// Booking is a seat reservation, either held (temporary) or confirmed.
type Booking struct {
	ID        string    `json:"id"`
	MovieID   string    `json:"movie_id"`
	SeatID    string    `json:"seat_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Store interface {
	Hold(ctx context.Context, b Booking) (Booking, error)
	List(ctx context.Context, movieID string) ([]Booking, error)
	Confirm(ctx context.Context, sessionID, userID string) (Booking, error)
	Release(ctx context.Context, sessionID, userID string) error
}
