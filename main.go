package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()

	server := http.Server{}
	server.Handler = mux
	server.Addr = ":8080"

	fileserver := http.FileServer(http.Dir("."))

	healthzHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}

	mux.Handle("/app/", http.StripPrefix("/app", fileserver))
	mux.HandleFunc("/healthz", healthzHandler)

	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("Error starting server: %v", "err")
		os.Exit(1)
	}
}
