package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

var db *sql.DB

type Point struct {
	RecordedAt string  `json:"recorded_at"`
	SensorID   string  `json:"sensor_id"`
	Value      float64 `json:"value"`
}

func queryReadings(table string) ([]Point, error) {
	rows, err := db.Query(
		"SELECT sensor_id, value, recorded_at FROM "+table+
			" ORDER BY recorded_at DESC LIMIT 500",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []Point
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.SensorID, &p.Value, &p.RecordedAt); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, nil
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
		dsn = "sensor:sensor@tcp(localhost:3306)/sensordb?parseTime=true"
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

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/api/temperature", apiHandler("temperatures"))
	http.HandleFunc("/api/humidity", apiHandler("humidities"))
	http.HandleFunc("/api/co2", apiHandler("co2s"))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
