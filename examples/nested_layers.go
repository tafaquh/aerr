package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/tafaquh/aerr"
)

func main() {
	// Setup slog default logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// Simulate an API request
	err := HandleUserRequest("12345")
	if err != nil {
		slog.Error("request failed", slog.Any("err", err))
	}
}

// HandleUserRequest simulates a controller/handler layer
func HandleUserRequest(userID string) error {
	err := GetUserService(userID)
	if err != nil {
		return aerr.Code("CONTROLLER_ERROR").
			Message("failed to handle user request").
			With("endpoint", "/api/users/"+userID).
			With("method", "GET").
			Wrap(err)
	}
	return nil
}

// GetUserService simulates a service layer
func GetUserService(userID string) error {
	err := FindUserRepository(userID)
	if err != nil {
		return aerr.Code("SERVICE_ERROR").
			Message("user service failed").
			With("service", "UserService").
			With("operation", "GetUser").
			Wrap(err)
	}
	return nil
}

// FindUserRepository simulates a repository layer
func FindUserRepository(userID string) error {
	err := QueryDatabase("SELECT * FROM users WHERE id = ?", userID)
	if err != nil {
		return aerr.Code("REPOSITORY_ERROR").
			Message("failed to find user in repository").
			StackTrace().
			With("user_id", userID).
			With("table", "users").
			Wrap(err)
	}
	return nil
}

// QueryDatabase simulates a database layer
func QueryDatabase(query string, args ...any) error {
	// Simulate a database connection error
	dbErr := errors.New("connection timeout after 5s")

	return aerr.Code("DB_ERROR").
		Message("database query failed").
		StackTrace().
		With("query", query).
		With("args", args).
		With("driver", "postgres").
		Err(dbErr)
}
