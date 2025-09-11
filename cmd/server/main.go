package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"

    "github.com/Tmacphee13/NanachiGo/internal/auth"
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
    if pw := os.Getenv("ADMIN_PASSWORD"); pw != "" {
        masked := strings.Repeat("*", len(pw))
        fmt.Println("ADMIN_PASSWORD loaded:", masked)
    } else {
        fmt.Println("ADMIN_PASSWORD not set; will default to 'admin'")
    }

    // Optional diagnostics and preflight checks when DEBUG is enabled
    debug := strings.EqualFold(os.Getenv("DEBUG"), "1") || strings.EqualFold(os.Getenv("DEBUG"), "true") || strings.EqualFold(os.Getenv("DEBUG"), "yes")
    if debug {
        fmt.Println("DEBUG enabled: running startup diagnostics")
        fmt.Println("DEFAULT_PLATFORM:", os.Getenv("DEFAULT_PLATFORM"))
        if r := os.Getenv("AWS_REGION"); r == "" {
            fmt.Println("AWS_REGION not set")
        } else {
            hasKey := os.Getenv("AWS_ACCESS_KEY_ID") != ""
            hasSecret := os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
            hasSession := os.Getenv("AWS_SESSION_TOKEN") != ""
            fmt.Printf("AWS configured (region=%s key=%t secret=%t session_token=%t)\n", r, hasKey, hasSecret, hasSession)
        }
        if pid := os.Getenv("GCP_PROJECT_ID"); pid == "" {
            fmt.Println("GCP_PROJECT_ID not set")
        } else {
            adc := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
            if adc == "" {
                fmt.Println("GCP credentials: GOOGLE_APPLICATION_CREDENTIALS not set (using ADC if available)")
            } else if _, err := os.Stat(adc); err != nil {
                fmt.Printf("GCP credentials: GOOGLE_APPLICATION_CREDENTIALS missing (%v)\n", err)
            } else {
                fmt.Printf("GCP credentials file: %s\n", adc)
            }
        }

        // Preflight checks (attempt both; they log and fail independently)
        ctx := context.Background()
        if cfg, err := auth.GetAWSConfig(); err != nil {
            log.Printf("preflight: aws config error: %v", err)
        } else {
            auth.TestAuthentication(cfg)
            if err := db.PreflightDynamoDB(ctx); err != nil {
                log.Printf("preflight: dynamodb error: %v", err)
            }
        }
        if err := db.PreflightFirestore(ctx); err != nil {
            log.Printf("preflight: firestore error: %v", err)
        }
    }

	flag.Parse()

	// each route gets a handler that points to a function that handles that path
	http.HandleFunc("/", home)
	http.HandleFunc("/admin", admin)
	http.HandleFunc("/api/login", login.Login)
	http.HandleFunc("/api/mindmaps", db.GetAllMindmaps)
	// id-based routes and actions
	http.HandleFunc("/api/mindmaps/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// action subroutes
		if r.Method == http.MethodPost {
			switch {
			case strings.HasSuffix(path, "/redo-description"):
				utils.RedoDescriptionHandler(w, r)
				return
			case strings.HasSuffix(path, "/remake-subtree"):
				utils.RemakeSubtreeHandler(w, r)
				return
			case strings.HasSuffix(path, "/go-deeper"):
				utils.GoDeeperHandler(w, r)
				return
			}
		}
		// DELETE /api/mindmaps/{id}
		if r.Method == http.MethodDelete {
			db.DeleteMindmapHandler(w, r)
			return
		}
		http.NotFound(w, r)
	})
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
