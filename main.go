package main

import "net/http"

func main() {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(".")))
	var srv http.Server
	srv.Handler = mux
	srv.Addr = ":8080"
	srv.ListenAndServe()
}
