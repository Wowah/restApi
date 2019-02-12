package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type RestHandler struct {
	db        *sql.DB
	N         int32
	CurN      *int32
	WhiteList []string
}

//var curN int32 = 0

func (h RestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if *h.CurN+1 > h.N {
		http.Error(w, "channel bandwidth exhausted", 509)
		return
	}
	eventType := strings.SplitN(r.URL.Path, "/", 2)[1]
	isFound := false
	for _, it := range h.WhiteList {
		if it == eventType {
			isFound = true
			break
		}
	}
	if !isFound {
		http.Error(w, "Event "+eventType+" is undefined", http.StatusBadRequest)
		return
	}
	log.Println("Event " + eventType + " registration begin.")
	atomic.AddInt32(h.CurN, 1)
	query := "INSERT INTO events (event_type, date) VALUES (?, ?)"
	curTime := time.Now()
	_, err := h.db.Exec(query, &eventType, &curTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("Event " + eventType + " successfully registered"))
	atomic.AddInt32(h.CurN, -1)
	log.Println("Event " + eventType + " successfully registered.")
}

type StatisticHandler struct {
	db        *sql.DB
	WhiteList []string
}

func (h StatisticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	strDuration := r.FormValue("time")
	duration := 1
	var err error
	if strDuration != "" {
		duration, err = strconv.Atoi(strDuration)
		if err != nil {
			http.Error(w, "Parameter 'time' is incorrect", http.StatusBadRequest)
			return
		}
	}
	log.Println("Begin of statistics method. Parameter 'time': " + strconv.Itoa(duration))
	startDate := time.Unix(0, time.Now().Sub(time.Unix(int64(duration*3600), 0)).Nanoseconds()).UTC()
	query := "select event_type, count(*) from events where date >= ? group by event_type"
	rows, err := h.db.Query(query, &startDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	results := make(map[string]int)
	for _, it := range h.WhiteList {
		results[it] = 0
	}
	var eventType string
	var count int
	for rows.Next() {
		rows.Scan(&eventType, &count)
		results[eventType] = count
	}
	data, err := json.Marshal(results)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(data)
	log.Println("End of statistics method. Parameter 'time': " + strconv.Itoa(duration))
}
