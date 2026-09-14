package main

import (
	"log"
	"net/http"

	"github.com/Tesquo/legendary-octo-memory/internal/api"
	"github.com/Tesquo/legendary-octo-memory/internal/storage"
)

func main() {
	// INIT DB
	db, err := storage.InitDB()
	if err != nil {
		log.Fatal(err)
	}

	api.DB = db

	// INIT ROUTER
	router := api.NewRouter()

	// LAST, RUN SERVER
	log.Println("Server running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal(err)
	}

}
