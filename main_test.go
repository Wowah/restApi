package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type testCase struct {
	URL               string
	StatusCode        int
	ExpectedBody      string
	IsStatisticMethod bool
}

func initWork() (*sql.DB, error) {
	DSN := "root:12345@tcp(mysql:3306)/restdb?charset=utf8"
	db, err := sql.Open("mysql", DSN)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	query := "DROP TABLE IF EXISTS events"
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}
	query = "CREATE TABLE events (id int PRIMARY KEY AUTO_INCREMENT, event_type varchar(20), date DATETIME)"
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func TestHandlers(t *testing.T) {
	db, err := initWork()
	if err != nil {
		t.Errorf("Error while database initialization\n%v", err)
		return
	}
	var curN int32
	regHandler := RestHandler{
		db,
		10,
		&curN,
		[]string{"a", "b", "c"},
	}
	statHandler := StatisticHandler{
		db,
		[]string{"a", "b", "c"},
	}
	mux := http.NewServeMux()
	mux.Handle("/stat", statHandler)
	mux.Handle("/", regHandler)
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	testCases := []testCase{
		testCase{
			URL:        "/a",
			StatusCode: http.StatusOK,
		},
		testCase{
			URL:        "/b",
			StatusCode: http.StatusOK,
		},
		testCase{
			URL:        "/c",
			StatusCode: http.StatusOK,
		},
		testCase{
			URL:        "/d",
			StatusCode: http.StatusBadRequest,
		},
		testCase{
			URL:        "/a",
			StatusCode: http.StatusOK,
		},
		testCase{
			URL:        "/hello",
			StatusCode: http.StatusBadRequest,
		},
		testCase{
			URL:               "/stat",
			StatusCode:        http.StatusOK,
			ExpectedBody:      "{\"a\":2,\"b\":1,\"c\":1}",
			IsStatisticMethod: true,
		},
		testCase{
			URL:               "/stat?time=48",
			StatusCode:        http.StatusOK,
			ExpectedBody:      "{\"a\":2,\"b\":1,\"c\":1}",
			IsStatisticMethod: true,
		},
		testCase{
			URL:        "/b",
			StatusCode: http.StatusOK,
		},
		testCase{
			URL:               "/stat",
			StatusCode:        http.StatusOK,
			ExpectedBody:      "{\"a\":2,\"b\":2,\"c\":1}",
			IsStatisticMethod: true,
		},
		testCase{
			URL:        "/stat?time=Wrong",
			StatusCode: http.StatusBadRequest,
		},
	}
	for i, it := range testCases {
		req, err := http.NewRequest(http.MethodGet, testServer.URL+it.URL, nil)
		if err != nil {
			t.Errorf("Error in %d case! %v", i, err)
			return
		}
		client := http.Client{Timeout: time.Second}
		res, err := client.Do(req)
		if err != nil {
			t.Errorf("Error in %d case! %v", i, err)
			return
		}
		defer res.Body.Close()
		data, err := ioutil.ReadAll(res.Body)
		respBody := string(data)
		if err != nil {
			t.Errorf("Error in %d case! %v", i, err)
			return
		}
		if res.StatusCode != it.StatusCode {
			t.Errorf("Error in %d case! Wrong response status code. Expected: %d. Got: %d", i, it.StatusCode, res.StatusCode)
		}
		if it.IsStatisticMethod && it.ExpectedBody != respBody {
			t.Errorf("Error in %d case! Response body is incorrect. Expected: %v. Got: %v", i, it.ExpectedBody, respBody)
		}
	}
}

func TestThrottling(t *testing.T) {
	db, err := initWork()
	if err != nil {
		t.Errorf("Error while database initialization\n%v", err)
		return
	}
	var curN int32
	regHandler := RestHandler{
		db,
		10,
		&curN,
		[]string{"a", "b", "c"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		regHandler.ServeHTTP(w, r)
		if r.URL.String() == "/a" {
			atomic.AddInt32(&curN, 1)
			time.Sleep(2 * time.Second)
			atomic.AddInt32(&curN, -1)
		}
	})
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	ch := make(chan error, 10)
	defer close(ch)
	for i := 0; i < 10; i++ {
		go func(out chan error, ts *httptest.Server) {
			req, err := http.NewRequest(http.MethodGet, ts.URL+"/a", nil)
			if err != nil {
				out <- err
				return
			}
			client := http.Client{}
			res, err := client.Do(req)
			if err != nil {
				out <- err
				return
			}
			if res.StatusCode != http.StatusOK {
				out <- fmt.Errorf("Response status incorrect. Expected: %d. Got: %d", http.StatusOK, res.StatusCode)
				return
			}
			out <- nil
		}(ch, testServer)
	}
	time.Sleep(time.Second)
	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/b", nil)
	if err != nil {
		t.Errorf("Error! %v", err)
		return
	}
	client := http.Client{Timeout: time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Errorf("Error! %v", err)
		return
	}
	if res.StatusCode != 509 {
		t.Errorf("Error in throttling test. Expected status code: 509. Got: %d", res.StatusCode)
	}
	for i := 0; i < 10; i++ {
		err := <-ch
		if err != nil {
			t.Errorf("Error in one of 10 server requests.\n%v", err)
		}
	}
}

func BenchmarkHandler(b *testing.B) {
	db, err := initWork()
	if err != nil {
		log.Fatalf("Error while database initialization\n%v", err)
	}
	var curN int32
	regHandler := RestHandler{
		db,
		1000,
		&curN,
		[]string{"a", "b", "c"},
	}
	mux := http.NewServeMux()
	mux.Handle("/", regHandler)
	testServer := httptest.NewServer(mux)
	client := http.Client{}
	var req *http.Request
	defer testServer.Close()
	for i := 0; i < b.N; i++ {
		req, err = http.NewRequest(http.MethodGet, testServer.URL+"/a", nil)
		if err != nil {
			log.Printf("Error in benchmark! %v\n", err)
			continue
		}
		_, err = client.Do(req)
		if err != nil {
			log.Printf("Error in benchmark! %v\n", err)
			continue
		}
	}
}
