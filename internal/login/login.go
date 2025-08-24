package login

import (
	"encoding/json"
	"net/http"
)

const ADMINPASS = "admin"

type LoginRequest struct {
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func login(w http.ResponseWriter, r *http.Request) {

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var resp LoginResponse
	if req.Password == ADMINPASS {
		resp = LoginResponse{Success: true, Message: "Login successful"}
		w.WriteHeader(http.StatusOK)
	} else {
		resp = LoginResponse{Success: false, Message: "Invalid password"}
		w.WriteHeader(http.StatusUnauthorized)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

}
