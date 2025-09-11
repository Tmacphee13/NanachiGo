package login

import (
    "encoding/json"
    "log"
    "net/http"
    "os"
    "strings"
    "sync"
)

var (
    adminPass     string
    adminPassOnce sync.Once
)

func getAdminPass() string {
    adminPassOnce.Do(func() {
        v := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
        if v == "" {
            log.Printf("WARNING: ADMIN_PASSWORD not set; defaulting to 'admin'")
            v = "admin"
        }
        adminPass = v
    })
    return adminPass
}

type LoginRequest struct {
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func Login(w http.ResponseWriter, r *http.Request) {

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

    var resp LoginResponse
    if req.Password == getAdminPass() {
        resp = LoginResponse{Success: true, Message: "Login successful"}
        w.WriteHeader(http.StatusOK)
    } else {
		resp = LoginResponse{Success: false, Message: "Invalid password"}
		w.WriteHeader(http.StatusUnauthorized)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

}
