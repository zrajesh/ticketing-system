package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisStore layout:
//
//	seat:{movieID}:{seatID} -> booking JSON (has a TTL while held, none once confirmed)
//	session:{sessionID}     -> seat key, so a session can be resolved to its seat
type RedisStore struct {
	rdb     *redis.Client
	holdTTL time.Duration
}

func NewRedisStore(rdb *redis.Client, holdTTL time.Duration) *RedisStore {
	return &RedisStore{rdb: rdb, holdTTL: holdTTL}
}

func redisSeatKey(movieID, seatID string) string {
	return fmt.Sprintf("seat:%s:%s", movieID, seatID)
}

func redisSessionKey(sessionID string) string {
	return "session:" + sessionID
}

func (s *RedisStore) Hold(ctx context.Context, b Booking) (Booking, error) {
	b.ID = uuid.NewString()
	b.Status = StatusHeld
	b.ExpiresAt = time.Now().Add(s.holdTTL)

	payload, err := json.Marshal(b)
	if err != nil {
		return Booking{}, err
	}

	key := redisSeatKey(b.MovieID, b.SeatID)

	// NX makes the claim atomic: of any number of concurrent callers, only one wins.
	ok, err := s.rdb.SetNX(ctx, key, payload, s.holdTTL).Result()
	if err != nil {
		return Booking{}, err
	}
	if !ok {
		return Booking{}, ErrSeatTaken
	}

	if err := s.rdb.Set(ctx, redisSessionKey(b.ID), key, s.holdTTL).Err(); err != nil {
		s.rdb.Del(ctx, key)
		return Booking{}, err
	}
	return b, nil
}

func (s *RedisStore) List(ctx context.Context, movieID string) ([]Booking, error) {
	var out []Booking

	iter := s.rdb.Scan(ctx, 0, fmt.Sprintf("seat:%s:*", movieID), 0).Iterator()
	for iter.Next(ctx) {
		raw, err := s.rdb.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue // expired between SCAN and GET
		}
		var b Booking
		if err := json.Unmarshal([]byte(raw), &b); err != nil {
			continue
		}
		out = append(out, b)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// load resolves a session to its seat key and booking, checking ownership.
func (s *RedisStore) load(ctx context.Context, sessionID, userID string) (string, Booking, error) {
	key, err := s.rdb.Get(ctx, redisSessionKey(sessionID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", Booking{}, ErrSessionNotFound
	}
	if err != nil {
		return "", Booking{}, err
	}

	raw, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", Booking{}, ErrSessionNotFound
	}
	if err != nil {
		return "", Booking{}, err
	}

	var b Booking
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return "", Booking{}, err
	}
	if b.UserID != userID {
		return "", Booking{}, ErrNotSessionOwner
	}
	return key, b, nil
}

func (s *RedisStore) Confirm(ctx context.Context, sessionID, userID string) (Booking, error) {
	key, b, err := s.load(ctx, sessionID, userID)
	if err != nil {
		return Booking{}, err
	}

	b.Status = StatusConfirmed
	b.ExpiresAt = time.Time{}

	payload, err := json.Marshal(b)
	if err != nil {
		return Booking{}, err
	}

	// Rewrite without a TTL so the confirmed seat never expires.
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, key, payload, 0)
	pipe.Persist(ctx, redisSessionKey(sessionID))
	if _, err := pipe.Exec(ctx); err != nil {
		return Booking{}, err
	}
	return b, nil
}

func (s *RedisStore) Release(ctx context.Context, sessionID, userID string) error {
	key, _, err := s.load(ctx, sessionID, userID)
	if err != nil {
		return err
	}
	return s.rdb.Del(ctx, key, redisSessionKey(sessionID)).Err()
}
