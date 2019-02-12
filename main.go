package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
)

func main() {
	DSN := "root:12345@tcp(mysql:3306)/restdb?charset=utf8"
	db, err := sql.Open("mysql", DSN)
	if err != nil {
		log.Fatalln("Error! Unable to open database connection.\n" + err.Error())
	}
	err = db.Ping()
	if err != nil {
		log.Fatalln("Error! Database connection doesn't work.\n" + err.Error())
	}
	query := "CREATE TABLE IF NOT EXISTS events (id int PRIMARY KEY AUTO_INCREMENT, event_type varchar(20), date DATETIME)"
	_, err = db.Exec(query)
	if err != nil {
		log.Fatalln("Error while creating table 'events'.\n" + err.Error())
	}
	var curN int32
	WhiteList := []string{"a", "b", "c"}
	regHandler := RestHandler{
		db,
		3,
		&curN,
		WhiteList,
	}
	statHandler := StatisticHandler{
		db,
		WhiteList,
	}
	mux := http.NewServeMux()
	mux.Handle("/stat", statHandler)
	mux.Handle("/", regHandler)
	log.Println("Starting server at :8082")
	http.ListenAndServe(":8082", mux)
}
