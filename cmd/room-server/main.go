package main

import (
	"log"
	"net/http"

	"github.com/zjx20/littlefighterhub/internal/server"
)

func main() {
	s := server.NewServer()
	http.HandleFunc("/", s.HandleConnections)

	log.Println("http server started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
