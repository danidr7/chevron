package server

import (
	"github.com/gorilla/mux"
	"github.com/quan-to/chevron"
	"github.com/quan-to/chevron/database"
	"github.com/quan-to/chevron/models"
	"github.com/quan-to/chevron/vaultManager"
	"github.com/quan-to/slog"
	r "gopkg.in/rethinkdb/rethinkdb-go.v5"
	"net/http"
)

type TestsEndpoint struct {
	log slog.Instance
}

func MakeTestsEndpoint(log slog.Instance) *TestsEndpoint {
	if log == nil {
		log = slog.Scope("Tests")
	} else {
		log = log.SubScope("Tests")
	}

	return &TestsEndpoint{
		log: log,
	}
}

func (ge *TestsEndpoint) AttachHandlers(r *mux.Router) {
	r.HandleFunc("/ping", ge.ping)
}

func (ge *TestsEndpoint) checkExternal() bool {
	isHealthy := true

	if remote_signer.EnableRethinkSKS {
		conn := database.GetConnection()

		_, err := r.Expr(1).Run(conn)
		if err != nil {
			ge.log.Error(err)
			isHealthy = false
		}
	}

	if remote_signer.VaultStorage {
		vm := vaultManager.MakeVaultManager(remote_signer.KeyPrefix)
		health, err := vm.HealthStatus()

		if err != nil {
			ge.log.Error(err)
			isHealthy = false
		}

		if !health.Initialized || health.Sealed {
			ge.log.Info("Vault initialized? %t, is sealed? %t", health.Initialized, health.Sealed)
			isHealthy = false
		}
	}

	return isHealthy
}

func (ge *TestsEndpoint) ping(w http.ResponseWriter, r *http.Request) {
	isHealthy := ge.checkExternal()

	// Do not log here. This call will flood the log
	w.Header().Set("Content-Type", models.MimeText)

	if isHealthy {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("OK"))
	} else {
		w.WriteHeader(503)
		_, _ = w.Write([]byte("Service Unavailable"))
	}
}
