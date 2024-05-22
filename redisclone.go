package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var mu sync.Mutex

type NewQueue struct {
	items []string
}

type expirationData struct {
	value  string
	expiry time.Time
}

var dataStore map[string]expirationData
var queue map[string]NewQueue
var ch = make(chan string)
var data = make(chan string, 1)
var wait sync.WaitGroup

func deleteExpiredKeys() {
	for range time.Tick(10 * time.Second) {
		mu.Lock()
		for key, expData := range dataStore {
			if !expData.expiry.IsZero() && time.Now().After(expData.expiry) {
				delete(dataStore, key)
			}
		}
		mu.Unlock()
	}
}

func setHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	r.ParseForm()
	key := r.Form.Get("key")
	value := r.Form.Get("value")

	if key == "" || value == "" {
		http.Error(w, "SET : key value is Mandatory", http.StatusBadRequest)
		return
	}

	mu.Lock()
	dataStore[key] = expirationData{value: value}
	mu.Unlock()
	fmt.Fprintf(w, "The %s: %s is Added ", key, value)
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	r.ParseForm()
	key := r.Form.Get("key")

	if key == "" {
		http.Error(w, "GET : key is Mandatory", http.StatusBadRequest)
		return
	}

	mu.Lock()
	expData, ok := dataStore[key]
	mu.Unlock()
	if !ok {
		http.Error(w, "The key is not present in the dictionary", http.StatusNotFound)
		return
	}

	fmt.Fprintf(w, expData.value)
}

func setexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}
	//fmt.println(r)

	r.ParseForm()
	key := r.Form.Get("key")
	value := r.Form.Get("value")

	expirySeconds, err := strconv.Atoi(r.Form.Get("seconds"))

	if key == "" || value == "" || err != nil {
		http.Error(w, "SETEX : key , seconds and value is Mandatory ", http.StatusBadRequest)
		return
	}

	expiryTime := time.Now().Add(time.Duration(expirySeconds) * time.Second)

	mu.Lock()
	dataStore[key] = expirationData{value: value, expiry: expiryTime}
	mu.Unlock()
	fmt.Fprintf(w, "The %s: %s is added for Only %d Seconds", key, value, expirySeconds)
}

func lpushHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	r.ParseForm()
	key := r.Form.Get("key")
	value := r.Form.Get("value")
	log.Println(key)
	log.Println(value)

	if key == "" || value == "" {
		http.Error(w, " LPUSH : key and value is Mandatory ", http.StatusBadRequest)
		return
	}

	mu.Lock()
	ed, ok := queue[key]
	if !ok {
		ed = NewQueue{items: []string{value}}
		go func() {
			data <- value
		}()
	} else {
		if len(ed.items) == 0 {
			go func() {
				data <- value
			}()
		}
		ed.items = append(ed.items, value)
	}
	queue[key] = ed
	mu.Unlock()

	fmt.Fprintf(w, "Element %s Added to List : %s ", value, key)
}

func blpopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		handleOptions(w)
		return
	}

	r.ParseForm()
	key := r.Form.Get("key")
	timeout, err := strconv.Atoi(r.Form.Get("timeout"))

	if key == "" || err != nil {
		http.Error(w, "BLPOP : key and timeout Mandatory", http.StatusBadRequest)
		return
	}

	mu.Lock()
	list, exists := queue[key]
	mu.Unlock()

	if !exists || len(list.items) == 0 {
		go func() {
			select {
			case data := <-data:
				ch <- data
			}
		}()
	} else {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if len(list.items) == 1 {
				fmt.Println(<-data)
			}
		}()
		wait.Wait()

		item := list.items[0]
		list.items = list.items[1:]
		queue[key] = list
		fmt.Fprintf(w, "The value is %s", item)
		return
	}

	select {
	case <-time.After(time.Duration(timeout) * time.Second):
		fmt.Fprintf(w, "No element present in the list")

	case item := <-ch:
		queue[key] = NewQueue{items: []string{}}
		queue[key] = list
		fmt.Fprintf(w, "The value is %s", item)
	}
}

func handleOptions(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.WriteHeader(http.StatusNoContent)
}

func withCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		handler(w, r)
	}
}

func main() {
	dataStore = make(map[string]expirationData)
	queue = make(map[string]NewQueue)

	go deleteExpiredKeys()

	http.HandleFunc("/set", withCORS(setHandler))
	http.HandleFunc("/get", withCORS(getHandler))
	http.HandleFunc("/setex", withCORS(setexHandler))
	http.HandleFunc("/lpush", withCORS(lpushHandler))
	http.HandleFunc("/blpop", withCORS(blpopHandler))

	fmt.Println("HTTP server listening on port 8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}
