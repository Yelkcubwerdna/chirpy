package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	servemux := http.NewServeMux()

	server := http.Server{}
	server.Handler = servemux
	server.Addr = ":8080"

	fileserver := http.FileServer(http.Dir("."))

	servemux.Handle("/", fileserver)

	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("Error starting server: %v", "err")
		os.Exit(1)
	}
}
