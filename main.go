package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/joeshaw/envdecode"
)

type Config struct {
	ListenAddr string `env:"LISTEN_ADDR,default=:9001"`
}

var config *Config

func init() {
	var c Config
	err := envdecode.Decode(&c)
	if err != nil {
		log.Fatalf("Config parsing failed: %s", err)
	}
	config = &c
}

func main() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/api/store/{path:.*}", storeHandler)
	http.ListenAndServe(config.ListenAddr, router)
}

func storeHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	path := vars["path"]
	user := r.Header.Get("X-User")

	if path == "" || user == "" {
		log.Printf("Bad request: path=%q user=%q", path, user)
		// XXX na to existuje http.Error !!! :)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("Request: %s %s -> user: %s path: %s", r.Method, r.URL, user, path)

	p := filepath.Join("store", user, path)

	f, err := os.Open(p)
	if err != nil {
		log.Printf("Error opening file: %s", err)
		if err == os.ErrNotExist {
			http.NotFound(w, r)
		} else {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	_, err = io.Copy(w, f)
	if err != nil {
		log.Print(err)
	}
}
