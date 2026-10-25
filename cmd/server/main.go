package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rajesh/ticketing-system/internal/booking"
	"github.com/rajesh/ticketing-system/internal/httpx"
	"github.com/rajesh/ticketing-system/internal/platform/redisclient"
)

const holdTTL = 2 * time.Minute

type movie struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Rows        int    `json:"rows"`
	SeatsPerRow int    `json:"seats_per_row"`
}

var movies = []movie{
	{ID: "inception", Title: "Inception", Rows: 5, SeatsPerRow: 8},
	{ID: "dune", Title: "Dune: Part Two", Rows: 4, SeatsPerRow: 6},
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	rdb, err := redisclient.New(context.Background(), getenv("REDIS_ADDR", "localhost:6379"))
	if err != nil {
		log.Fatal(err)
	}

	handler := booking.NewHandler(booking.NewService(booking.NewRedisStore(rdb, holdTTL)))

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.Dir("static")))
	mux.HandleFunc("GET /movies", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, movies)
	})
	mux.HandleFunc("GET /movies/{movieID}/seats", handler.ListSeats)
	mux.HandleFunc("POST /movies/{movieID}/seats/{seatID}/hold", handler.HoldSeat)
	mux.HandleFunc("PUT /sessions/{sessionID}/confirm", handler.ConfirmSession)
	mux.HandleFunc("DELETE /sessions/{sessionID}", handler.ReleaseSession)

	addr := getenv("ADDR", ":8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
