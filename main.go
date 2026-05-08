package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

var db *sql.DB

type Point struct {
	RecordedAt time.Time `json:"recorded_at"`
	SensorID   string    `json:"sensor_id"`
	Value      float64   `json:"value"`
}

func queryReadings(table string) ([]Point, error) {
	rows, err := db.Query(
		"SELECT sensor_id, AVG(value), DATE_FORMAT(recorded_at, '%Y-%m-%d %H:00:00') FROM "+table+
			" WHERE recorded_at >= NOW() - INTERVAL 7 DAY"+
			" GROUP BY sensor_id, DATE_FORMAT(recorded_at, '%Y-%m-%d %H:00:00')"+
			" ORDER BY recorded_at ASC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []Point
	for rows.Next() {
		var p Point
		var t string
		if err := rows.Scan(&p.SensorID, &p.Value, &t); err != nil {
			return nil, err
		}
		p.RecordedAt, _ = time.ParseInLocation("2006-01-02 15:04:05", t, time.Local)
		points = append(points, p)
	}
	return points, nil
}

func basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, ok := r.BasicAuth()
		if !ok || pass != os.Getenv("VIEW_PASSWORD") {
			w.Header().Set("WWW-Authenticate", `Basic realm="sensor-view"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func apiHandler(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		points, err := queryReadings(table)
		if err != nil {
			log.Printf("query error: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(points)
	}
}

func main() {
	godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "sensor:sensor@tcp(localhost:3306)/sensordb?parseTime=true&loc=Asia%2FTokyo"
	}

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	if os.Getenv("VIEW_PASSWORD") == "" {
		log.Fatal("VIEW_PASSWORD is required")
	}

	http.Handle("/", basicAuth(http.FileServer(http.Dir("./static"))))
	http.Handle("/api/temperature", basicAuth(apiHandler("temperatures")))
	http.Handle("/api/humidity", basicAuth(apiHandler("humidities")))
	http.Handle("/api/co2", basicAuth(apiHandler("co2s")))
	http.Handle("/api/smell", basicAuth(apiHandler("smells")))

	srv := &http.Server{Addr: ":8080"}
	go func() {
		log.Println("listening on :8080")
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("server shutdown")
}
