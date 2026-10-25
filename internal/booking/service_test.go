package booking

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHold_OnlyOneConcurrentCallerWins(t *testing.T) {
	svc := NewService(NewMemoryStore(time.Minute))

	const callers = 1000
	var wins atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Hold(context.Background(), Booking{MovieID: "dune", SeatID: "A1", UserID: "u"})
			if err == nil {
				wins.Add(1)
			} else if !errors.Is(err, ErrSeatTaken) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", got)
	}
}

func TestConfirmAndRelease_RequireOwner(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(time.Minute))

	held, err := svc.Hold(ctx, Booking{MovieID: "dune", SeatID: "B2", UserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Confirm(ctx, held.ID, "bob"); !errors.Is(err, ErrNotSessionOwner) {
		t.Errorf("confirm by non-owner: got %v", err)
	}
	if err := svc.Release(ctx, held.ID, "bob"); !errors.Is(err, ErrNotSessionOwner) {
		t.Errorf("release by non-owner: got %v", err)
	}

	b, err := svc.Confirm(ctx, held.ID, "alice")
	if err != nil || b.Status != StatusConfirmed {
		t.Fatalf("confirm by owner: %v, %+v", err, b)
	}
}

func TestHold_ExpiresAfterTTL(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(10 * time.Millisecond))

	if _, err := svc.Hold(ctx, Booking{MovieID: "dune", SeatID: "C3", UserID: "alice"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)

	if _, err := svc.Hold(ctx, Booking{MovieID: "dune", SeatID: "C3", UserID: "bob"}); err != nil {
		t.Fatalf("seat should be free after expiry: %v", err)
	}
}
