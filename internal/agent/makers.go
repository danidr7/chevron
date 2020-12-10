package agent

import (
	"github.com/quan-to/chevron/internal/config"
	"github.com/quan-to/chevron/pkg/database/memory"
	"github.com/quan-to/chevron/pkg/database/pg"
	"github.com/quan-to/chevron/pkg/database/rql"
	"github.com/quan-to/chevron/pkg/interfaces"
	"github.com/quan-to/chevron/pkg/models"
	"github.com/quan-to/slog"
)

type DatabaseHandler interface {
	GetUser(username string) (um *models.User, err error)
	AddUserToken(ut models.UserToken) (string, error)
	RemoveUserToken(token string) (err error)
	GetUserToken(token string) (ut *models.UserToken, err error)
	InvalidateUserTokens() (int, error)
	AddUser(um models.User) (string, error)
	UpdateUser(um models.User) error
	AddGPGKey(key models.GPGKey) (string, bool, error)
	FindGPGKeyByEmail(email string, pageStart, pageEnd int) ([]models.GPGKey, error)
	FindGPGKeyByFingerPrint(fingerPrint string, pageStart, pageEnd int) ([]models.GPGKey, error)
	FindGPGKeyByValue(value string, pageStart, pageEnd int) ([]models.GPGKey, error)
	FindGPGKeyByName(name string, pageStart, pageEnd int) ([]models.GPGKey, error)
	FetchGPGKeyByFingerprint(fingerprint string) (*models.GPGKey, error)
	HealthCheck() error
}

func makeRethinkDBHandler(logger slog.Instance) (*rql.RethinkDBDriver, error) {
	logger.Info("RethinkDB Database Enabled. Creating handler")
	rdb := rql.MakeRethinkDBDriver(logger)
	logger.Info("Connecting to RethinkDB")
	err := rdb.Connect(config.RethinkDBHost, config.RethinkDBUsername, config.RethinkDBPassword, config.DatabaseName, config.RethinkDBPort, config.RethinkDBPoolSize)
	if err != nil {
		return nil, err
	}
	logger.Info("Initializing database")
	err = rdb.InitDatabase()
	if err != nil {
		return nil, err
	}
	logger.Info("RethinkDB Handler done!")
	return rdb, nil
}

func makePostgresDBHandler(logger slog.Instance) (*pg.PostgreSQLDBDriver, error) {
	logger.Info("PostgreSQL Database Enabled. Creating handler")
	rdb := pg.MakePostgreSQLDBDriver(logger)
	logger.Info("Initializing database")
	err := rdb.Connect(config.ConnectionString)
	if err != nil {
		return nil, err
	}
	return rdb, nil
}

// MakeDatabaseHandler initializes a Database Access Handler based on the current configuration
func MakeDatabaseHandler(logger slog.Instance) (DatabaseHandler, error) {
	if config.EnableDatabase {
		switch config.DatabaseDialect {
		case "rethinkdb":
			return makeRethinkDBHandler(logger)
		case "postgres":
			return makePostgresDBHandler(logger)
		case "memory":
			return memory.MakeMemoryDBDriver(logger), nil
		default:
			logger.Fatal("Unknown database dialect %q", config.DatabaseDialect)
		}
	}
	logger.Warn("No database handler enabled. Using memory database")

	return memory.MakeMemoryDBDriver(logger), nil
}

// MakeTokenManager creates an instance of token manager. If Rethink is enabled returns an DatabaseTokenManager, if not a MemoryTokenManager
func MakeTokenManager(logger slog.Instance, dbHandler DatabaseHandler) interfaces.TokenManager {
	if dbHandler != nil {
		return MakeDatabaseTokenManager(logger, dbHandler)
	}

	return MakeMemoryTokenManager(logger)
}

// MakeAuthManager creates an instance of auth manager. If Rethink is enabled returns an DatabaseAuthManager, if not a JSONAuthManager
func MakeAuthManager(logger slog.Instance, dbHandler DatabaseHandler) interfaces.AuthManager {
	if dbHandler != nil {
		return NewDatabaseAuthManager(logger, dbHandler)
	}

	return MakeJSONAuthManager(logger)
}
