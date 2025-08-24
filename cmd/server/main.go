package main

import (
	"log"
	//"net/http"

	//"github.com/Tmacphee13/mindmap_go/internal/auth"
	//"github.com/Tmacphee13/mindmap_go/internal/server"
	"github.com/Tmacphee13/mindmap_go/internal/db"
	"github.com/joho/godotenv"
)

func main() {

	// LoadEnv loads environment variables from the .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	// testing authenticatio
	/*
		config := auth.GetAWSConfig()
		auth.TestAuthentication(config)
	*/

	// testing server initializaton
	/*
		srv := server.New()

		log.Println("Server running on :8080")
		if err := http.ListenAndServe(":8080", srv.Router()); err != nil {
			log.Fatal(err)
		}
	*/

	// testing dynamodb connection
	db.ListDynamoDBTables()

}
