package server

import (
	"net/http"
)

type Server struct{}

func New() *Server {
	return &Server{}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// serve static files from public
	fs := http.FileServer(http.Dir("public"))
	mux.Handle("/", fs)

	// Example API endpoint
	mux.Handle("/api/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Status":"ok"}`))
	}))

	return mux
}
