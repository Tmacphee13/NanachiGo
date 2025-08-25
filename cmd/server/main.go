package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/Tmacphee13/NanachiGo/internal/db"
	"github.com/Tmacphee13/NanachiGo/internal/login"
	"github.com/Tmacphee13/NanachiGo/internal/utils"
	"github.com/joho/godotenv"
)

/*
// testing authenticatio
config := auth.GetAWSConfig()
auth.TestAuthentication(config)

// testing dynamodb connection
db.ListDynamoDBTables()
*/

func main() {

	// LoadEnv loads environment variables from the .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	flag.Parse()

	// each route gets a handler that points to a function that handles that path
	http.HandleFunc("/", home)
	http.HandleFunc("/admin", admin)
	http.HandleFunc("/api/login", login.Login)
	http.HandleFunc("/api/mindmaps", db.GetAllMindmaps)
	http.HandleFunc("/api/upload", utils.UploadPaper)
	http.ListenAndServe(":3000", nil)
	//http.ListenAndServe(*addr, nil)
}

// --------------------- Handler Funcs --------------------------//
func home(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./public/index.html")
}

func admin(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./public/admin.html")
}

/* #------------ Imported Functions ------------#
login.Login


*/
