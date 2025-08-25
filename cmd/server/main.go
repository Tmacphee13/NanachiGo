package main

import (
	"flag"
	//"log"
	"net/http"
	//"github.com/joho/godotenv"
	"github.com/Tmacphee13/NanachiGo/internal/login"
)

/*
// testing authenticatio
config := auth.GetAWSConfig()
auth.TestAuthentication(config)

// testing server initializaton
srv := server.New()

log.Println("Server running on :8080")
if err := http.ListenAndServe(":8080", srv.Router()); err != nil {
	log.Fatal(err)
}

// testing dynamodb connection
db.ListDynamoDBTables()
*/

func main() {

	// LoadEnv loads environment variables from the .env file
	/*
		err := godotenv.Load()
		if err != nil {
			log.Fatalf("Error loading .env file: %v", err)
		}
	*/

	flag.Parse()

	// each route gets a handler that points to a function that handles that path
	http.HandleFunc("/", home)
	http.HandleFunc("/admin", admin)
	http.HandleFunc("/api/login", login.Login)
	//http.ListenAndServe(*addr, nil)
	http.ListenAndServe(":3000", nil)
}

// --------------------- Handler Funcs --------------------------//
func home(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./public/index.html")
}

func admin(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./public/admin.html")
}

/*
func getMindMaps(w http.ResponseWriter, r *http.Request) {

}
*/
