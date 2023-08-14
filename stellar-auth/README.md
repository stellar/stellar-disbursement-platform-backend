# Stellar Auth

Stellar Auth is a package that provides authentication functionality for Stellar applications. It simplifies the process of managing user authentication.

## Table of Contents

- [CLI](#cli)
	- [add-user](#add-user)
	- [roles](#roles)
- [Usage](#usage)

## CLI

The Stellar Auth provides a CLI that helps adding new users and applying the database migrations in order to create all necessary tables.

```sh
Stellar Auth handles JWT management.

Usage:
  stellarauth [flags]
  stellarauth [command]

Available Commands:
  add-user    Add user to the system
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  migrate     Apply Stellar Auth database migrations

Flags:
      --database-url string   Postgres DB URL (DATABASE_URL) (default "postgres://postgres:postgres@localhost:5432/stellar-auth?sslmode=disable")
  -h, --help                  help for stellarauth
      --log-level string      The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC". (LOG_LEVEL) (default "TRACE")

Use "stellarauth [command] --help" for more information about a command.
```

### add-user

To add a new user using the CLI you can use the `add-user` subcommand.

```sh
$ stellarauth add-user --help

Usage:
  stellarauth add-user <email> <first name> <last name> [--owner] [--roles] [--password] [flags]

Flags:
  -h, --help       help for add-user
      --owner      Set the user as Owner (superuser). Defaults to "false". (OWNER)
      --password   Sets the user password, it should be at least 8 characters long, if omitted, the command will generate a random one. (PASSWORD)

Global Flags:
      --database-url string   Postgres DB URL (DATABASE_URL) (default "postgres://postgres:postgres@localhost:5432/stellar-auth?sslmode=disable")
      --log-level string      The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC". (LOG_LEVEL) (default "TRACE")
```

When creating a new user you can set the password.

```sh
$ export DATABASE_URL=postgres://... # Or you can specify in the command --database-url postgres://..
$ stellarauth migrate up # Creating the necessary tables
$ stellarauth add-user mary.jane@stellar.org Mary Jane --password

INFO[2023-07-31T17:05:46.292-03:00] Version: 0.2.0                                pid=22464
INFO[2023-07-31T17:05:46.292-03:00] GitCommit:                                    pid=22464
Password:         
INFO[2023-07-31T17:05:55.159-03:00] user inserted: mary.jane@stellar.org          pid=22464
```

### roles

You can add role management by passing the available roles to the `AddUserCmd`. After this the flag `--roles` will show up in the `add-user` subcommand.

```go
// pkg/cli/root.go

func SetupCLI(version, gitCommit string) *cobra.Command {
	// ...

	cmd.AddCommand(AddUserCmd("", NewDefaultPasswordPrompt(), []string{"approver", "editor", "owner"}))

	return cmd
}

```

```sh
$ stellarauth add-user --help

Usage:
  stellarauth add-user <email> <first name> <last name> [--owner] [--roles] [--password] [flags]

Flags:
  -h, --help       help for add-user
      --owner      Set the user as Owner (superuser). Defaults to "false". (OWNER)
      --password   Sets the user password, it should be at least 8 characters long, if omitted, the command will generate a random one. (PASSWORD)
	  --roles string   Set the user roles. It should be comma separated. Example: role1, role2. Available roles: [approver, editor, owner]. (ROLES)

Global Flags:
      --database-url string   Postgres DB URL (DATABASE_URL) (default "postgres://postgres:postgres@localhost:5432/stellar-auth?sslmode=disable")
      --log-level string      The log level used in this project. Options: "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", or "PANIC". (LOG_LEVEL) (default "TRACE")
```

```sh
$ stellarauth add-user mary.jane@stellar.org Mary Jane --roles approver,editor --password

INFO[2023-07-31T17:05:46.292-03:00] Version: 0.2.0                                pid=22464
INFO[2023-07-31T17:05:46.292-03:00] GitCommit:                                    pid=22464
Password:         
INFO[2023-07-31T17:05:55.159-03:00] user inserted: mary.jane@stellar.org          pid=22464
```

## Usage

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	authdb "github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

var AuthManager auth.AuthManager

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func main() {
	mux := http.NewServeMux()

	databaseURL := os.Getenv("DATABASE_URL")
	dbConnectionPool, err := authdb.OpenDBConnectionPool(databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	// Instantiating AuthManager using the default options
	AuthManager = auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(dbConnectionPool, auth.NewDefaultPasswordEncrypter(), time.Hour*1),
		auth.WithDefaultJWTManagerOption(os.Getenv("EC256_PUBLIC_KEY"), os.Getenv("DATABASE_URL")),
		auth.WithDefaultRoleManagerOption(dbConnectionPool, "owner"),
	)

	mux.HandleFunc("/login", login)
	mux.HandleFunc("/refresh-token", refreshToken)
	mux.Handle("/authenticated", AuthenticatedMiddleware(http.HandlerFunc(myAuthenticatedHandler)))
	mux.Handle("/role-required", AuthenticatedMiddleware(
		RoleMiddleware([]string{"myRole1", "myRole2"})(http.HandlerFunc(myRoleRequiredHandler)),
	))

	http.ListenAndServe(":8000", mux)
}

func myAuthenticatedHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`Ok`))
}

func myRoleRequiredHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`Ok`))
}

func login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var reqBody LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid request body"}`))
	}

	token, err := AuthManager.Authenticate(ctx, reqBody.Email, reqBody.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte(fmt.Sprintf(`{"token": %q}`, token)))
}

func refreshToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := r.Header.Get("Authorization")

	token, err := AuthManager.RefreshToken(ctx, token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte(fmt.Sprintf(`{"token": %q}`, token)))
}

func AuthenticatedMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token := r.Header.Get("Authorization")

		// Does the header validation...

		isValid, err := AuthManager.ValidateToken(ctx, token)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "not authorized"}`))
			return
		}

		if !isValid {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "not authorized"}`))
			return
		}

		// Additionally you can add the token to the request context
		ctx = context.WithValue(ctx, "tokenKey", token)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

func RoleMiddleware(requiredRoles []string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			token := r.Header.Get("Authorization")

			// Does the header validation...

			hasAnyRoles, err := AuthManager.AnyRolesInTokenUser(ctx, token, requiredRoles)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "not authorized"}`))
				return
			}

			if !hasAnyRoles {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "not authorized"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
```
