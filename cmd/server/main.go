package main

import (
	"log"
	"net/http"

	"github.com/Tesquo/legendary-octo-memory/internal/api"
)

func main() {
	router := api.NewRouter()

	log.Println("Server running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal(err)
	}
}
