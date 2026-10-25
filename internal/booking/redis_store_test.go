package booking

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rajesh/ticketing-system/internal/platform/redisclient"
)

// newTestRedisStore skips the test when Redis isn't reachable, so `go test`
// still passes on a machine without it.
func newTestRedisStore(t *testing.T, ttl time.Duration) *RedisStore {
	t.Helper()

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb, err := redisclient.New(context.Background(), addr)
	if err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })

	return NewRedisStore(rdb, ttl)
}

func TestRedisStore_OnlyOneConcurrentHoldWins(t *testing.T) {
	store := newTestRedisStore(t, time.Minute)
	movie := "test-" + uuid.NewString()

	const callers = 2000
	var wins, taken atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Hold(context.Background(), Booking{MovieID: movie, SeatID: "A1", UserID: uuid.NewString()})
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, ErrSeatTaken):
				taken.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if wins.Load() != 1 || taken.Load() != callers-1 {
		t.Fatalf("wins=%d taken=%d, want 1 and %d", wins.Load(), taken.Load(), callers-1)
	}
}

func TestRedisStore_Lifecycle(t *testing.T) {
	ctx := context.Background()
	store := newTestRedisStore(t, time.Minute)
	movie := "test-" + uuid.NewString()

	held, err := store.Hold(ctx, Booking{MovieID: movie, SeatID: "B2", UserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Confirm(ctx, held.ID, "bob"); !errors.Is(err, ErrNotSessionOwner) {
		t.Fatalf("confirm by non-owner: got %v", err)
	}
	if _, err := store.Confirm(ctx, held.ID, "alice"); err != nil {
		t.Fatal(err)
	}

	// A confirmed seat must lose its TTL.
	if ttl := store.rdb.TTL(ctx, redisSeatKey(movie, "B2")).Val(); ttl != -1 {
		t.Fatalf("confirmed seat ttl = %v, want -1 (no expiry)", ttl)
	}

	list, err := store.List(ctx, movie)
	if err != nil || len(list) != 1 || list[0].Status != StatusConfirmed {
		t.Fatalf("list = %+v, err = %v", list, err)
	}

	if err := store.Release(ctx, held.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Hold(ctx, Booking{MovieID: movie, SeatID: "B2", UserID: "bob"}); err != nil {
		t.Fatalf("seat should be free after release: %v", err)
	}
}

func TestRedisStore_HoldExpires(t *testing.T) {
	ctx := context.Background()
	store := newTestRedisStore(t, time.Second)
	movie := "test-" + uuid.NewString()

	held, err := store.Hold(ctx, Booking{MovieID: movie, SeatID: "C3", UserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)

	if _, err := store.Confirm(ctx, held.ID, "alice"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("confirm after expiry: got %v", err)
	}
	if _, err := store.Hold(ctx, Booking{MovieID: movie, SeatID: "C3", UserID: "bob"}); err != nil {
		t.Fatalf("seat should be free after expiry: %v", err)
	}
}
