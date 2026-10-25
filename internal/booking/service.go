package booking

import "context"

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Hold(ctx context.Context, b Booking) (Booking, error) {
	return s.store.Hold(ctx, b)
}

func (s *Service) List(ctx context.Context, movieID string) ([]Booking, error) {
	return s.store.List(ctx, movieID)
}

func (s *Service) Confirm(ctx context.Context, sessionID, userID string) (Booking, error) {
	return s.store.Confirm(ctx, sessionID, userID)
}

func (s *Service) Release(ctx context.Context, sessionID, userID string) error {
	return s.store.Release(ctx, sessionID, userID)
}
