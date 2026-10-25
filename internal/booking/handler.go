package booking

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/rajesh/ticketing-system/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type userRequest struct {
	UserID string `json:"user_id"`
}

type sessionResponse struct {
	SessionID string `json:"session_id"`
	MovieID   string `json:"movie_id"`
	SeatID    string `json:"seat_id"`
	UserID    string `json:"user_id"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type seatResponse struct {
	SeatID    string `json:"seat_id"`
	UserID    string `json:"user_id"`
	Booked    bool   `json:"booked"`
	Confirmed bool   `json:"confirmed"`
}

func decodeUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req userRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		httpx.Error(w, http.StatusBadRequest, "user_id is required")
		return "", false
	}
	return req.UserID, true
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSeatTaken):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrSessionNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrNotSessionOwner):
		httpx.Error(w, http.StatusForbidden, err.Error())
	default:
		log.Printf("booking: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *Handler) ListSeats(w http.ResponseWriter, r *http.Request) {
	bookings, err := h.svc.List(r.Context(), r.PathValue("movieID"))
	if err != nil {
		writeStoreError(w, err)
		return
	}

	seats := make([]seatResponse, 0, len(bookings))
	for _, b := range bookings {
		seats = append(seats, seatResponse{
			SeatID:    b.SeatID,
			UserID:    b.UserID,
			Booked:    true,
			Confirmed: b.Status == StatusConfirmed,
		})
	}
	httpx.JSON(w, http.StatusOK, seats)
}

func (h *Handler) HoldSeat(w http.ResponseWriter, r *http.Request) {
	userID, ok := decodeUser(w, r)
	if !ok {
		return
	}

	held, err := h.svc.Hold(r.Context(), Booking{
		MovieID: r.PathValue("movieID"),
		SeatID:  r.PathValue("seatID"),
		UserID:  userID,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, sessionResponse{
		SessionID: held.ID,
		MovieID:   held.MovieID,
		SeatID:    held.SeatID,
		UserID:    held.UserID,
		Status:    held.Status,
		ExpiresAt: held.ExpiresAt.Format(time.RFC3339),
	})
}

func (h *Handler) ConfirmSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := decodeUser(w, r)
	if !ok {
		return
	}

	b, err := h.svc.Confirm(r.Context(), r.PathValue("sessionID"), userID)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	httpx.JSON(w, http.StatusOK, sessionResponse{
		SessionID: b.ID,
		MovieID:   b.MovieID,
		SeatID:    b.SeatID,
		UserID:    b.UserID,
		Status:    b.Status,
	})
}

func (h *Handler) ReleaseSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := decodeUser(w, r)
	if !ok {
		return
	}

	if err := h.svc.Release(r.Context(), r.PathValue("sessionID"), userID); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
