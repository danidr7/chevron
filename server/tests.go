package server

import (
	"github.com/gorilla/mux"
	"github.com/quan-to/chevron/models"
	"net/http"
)

type TestsEndpoint struct{}

func MakeTestsEndpoint() *TestsEndpoint {
	return &TestsEndpoint{}
}

func (ge *TestsEndpoint) AttachHandlers(r *mux.Router) {
	r.HandleFunc("/ping", ge.ping)
}

func (ge *TestsEndpoint) ping(w http.ResponseWriter, r *http.Request) {
	// Do not log here. This call will flood the log
	w.Header().Set("Content-Type", models.MimeText)
	w.WriteHeader(200)
	_, _ = w.Write([]byte("OK"))
}
