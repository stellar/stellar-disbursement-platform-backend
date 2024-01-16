package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type PasswordPromptMock struct{}

func (m *PasswordPromptMock) Run() (string, error) {
	return "!1Az?2By.3Cx", nil
}

var _ PasswordPromptInterface = (*PasswordPromptMock)(nil)

func Test_authAddUserCommand(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	mockPrompt := PasswordPromptMock{}
	mockedPassword, _ := mockPrompt.Run()

	t.Run("Should create a new user", func(t *testing.T) {
		addUser := AddUserCmd("database-url", &mockPrompt, []string{})
		rootCmd := rootCmd()
		rootCmd.AddCommand(addUser)

		newEmail := "newuser@email.com"
		firstName := "first"
		lastName := "last"
		rootCmd.SetArgs([]string{"--database-url", dbt.DSN, "add-user", newEmail, firstName, lastName, "--password"})
		err := rootCmd.Execute()
		require.NoError(t, err)

		var dbEmail, dbPassword, dbFirstName, dbLastName string
		var dbIsOwner bool
		err = dbConnectionPool.QueryRowxContext(ctx, "SELECT email, encrypted_password, is_owner, first_name, last_name FROM auth_users WHERE email = $1", newEmail).Scan(&dbEmail, &dbPassword, &dbIsOwner, &dbFirstName, &dbLastName)
		require.NoError(t, err)

		assert.Equal(t, newEmail, dbEmail)
		assert.NotEqual(t, dbPassword, mockedPassword)
		assert.False(t, dbIsOwner)
		assert.Equal(t, firstName, dbFirstName)
		assert.Equal(t, lastName, dbLastName)
	})

	t.Run("Should create a new Owner user", func(t *testing.T) {
		addUser := AddUserCmd("database-url", &mockPrompt, []string{})
		rootCmd := rootCmd()
		rootCmd.AddCommand(addUser)

		newEmail := "newuserowner@email.com"
		firstName := "first"
		lastName := "last"
		rootCmd.SetArgs([]string{"--database-url", dbt.DSN, "add-user", newEmail, firstName, lastName, "--password", "--owner"})
		err := rootCmd.Execute()
		require.NoError(t, err)

		var dbEmail, dbPassword, dbFirstName, dbLastName string
		var dbIsOwner bool
		err = dbConnectionPool.QueryRowxContext(ctx, "SELECT email, encrypted_password, is_owner, first_name, last_name FROM auth_users WHERE email = $1", newEmail).Scan(&dbEmail, &dbPassword, &dbIsOwner, &dbFirstName, &dbLastName)
		require.NoError(t, err)

		assert.Equal(t, newEmail, dbEmail)
		assert.NotEqual(t, dbPassword, mockedPassword)
		assert.True(t, dbIsOwner)
		assert.Equal(t, firstName, dbFirstName)
		assert.Equal(t, lastName, dbLastName)
	})

	t.Run("Should create a new user with random generated password", func(t *testing.T) {
		addUser := AddUserCmd("database-url", &mockPrompt, []string{})
		rootCmd := rootCmd()
		rootCmd.AddCommand(addUser)

		newEmail := "newuserpass@email.com"
		firstName := "first"
		lastName := "last"
		rootCmd.SetArgs([]string{"--database-url", dbt.DSN, "add-user", newEmail, firstName, lastName})
		err := rootCmd.Execute()
		require.NoError(t, err)

		var dbEmail, dbPassword, dbFirstName, dbLastName string
		var dbIsOwner bool
		err = dbConnectionPool.QueryRowxContext(ctx, "SELECT email, encrypted_password, is_owner, first_name, last_name FROM auth_users WHERE email = $1", newEmail).Scan(&dbEmail, &dbPassword, &dbIsOwner, &dbFirstName, &dbLastName)
		require.NoError(t, err)

		assert.Equal(t, newEmail, dbEmail)
		assert.NotEmpty(t, dbPassword)
		assert.False(t, isOwner)
	})

	t.Run("should show the correct usage", func(t *testing.T) {
		setTestCmd := func() *cobra.Command {
			return &cobra.Command{
				Use: "test",
			}
		}

		addUserCmd := AddUserCmd("database-url", &mockPrompt, []string{})

		buf := new(strings.Builder)
		testCmd := setTestCmd()
		testCmd.SetOut(buf)
		testCmd.AddCommand(addUserCmd)

		testCmd.SetArgs([]string{"add-user", "--help"})
		err := testCmd.Execute()
		require.NoError(t, err)

		expectedUsage := fmt.Sprintf(`Add a user to the system. Email should be unique and password must be at least %d characters long.

Usage:
  test add-user <email> <first name> <last name> [--owner] [--roles] [--password] [flags]

Flags:
  -h, --help       help for add-user
      --owner      Set the user as Owner (superuser). Defaults to "false". (OWNER)
      --password   Sets the user password, it should be at least %d characters long, if omitted, the command will generate a random one. (PASSWORD)
`, auth.MinPasswordLength, auth.MinPasswordLength)
		assert.Equal(t, expectedUsage, buf.String())

		addUserCmd = AddUserCmd("database-url", &mockPrompt, []string{"role1", "role2", "role3", "role4"})

		buf = new(strings.Builder)
		testCmd = setTestCmd()
		testCmd.SetOut(buf)
		testCmd.AddCommand(addUserCmd)

		testCmd.SetArgs([]string{"add-user", "--help"})
		err = testCmd.Execute()
		require.NoError(t, err)

		expectedUsage = fmt.Sprintf(`Add a user to the system. Email should be unique and password must be at least %d characters long.

Usage:
  test add-user <email> <first name> <last name> [--owner] [--roles] [--password] [flags]

Flags:
  -h, --help           help for add-user
      --owner          Set the user as Owner (superuser). Defaults to "false". (OWNER)
      --password       Sets the user password, it should be at least %d characters long, if omitted, the command will generate a random one. (PASSWORD)
      --roles string   Set the user roles. It should be comma separated. Example: role1, role2. Available roles: [role1, role2, role3, role4]. (ROLES)
`, auth.MinPasswordLength, auth.MinPasswordLength)
		assert.Equal(t, expectedUsage, buf.String())
	})

	t.Run("set the user roles", func(t *testing.T) {
		rootCmd := rootCmd()
		addUserCmd := AddUserCmd("database-url", &mockPrompt, []string{"role1", "role2"})
		rootCmd.AddCommand(addUserCmd)

		buf := new(strings.Builder)
		rootCmd.SetOut(buf)

		email, firstName, lastName := "test@email.com", "First", "Last"

		rootCmd.SetArgs([]string{"--database-url", dbt.DSN, "add-user", email, firstName, lastName, "--roles", "role2"})
		err := rootCmd.Execute()
		require.NoError(t, err)

		var dbUsername, dbFirstName, dbLastName string
		var dbRoles []string
		err = dbConnectionPool.QueryRowxContext(ctx, "SELECT email, first_name, last_name, roles FROM auth_users WHERE email = $1", email).Scan(&dbUsername, &dbFirstName, &dbLastName, pq.Array(&dbRoles))
		require.NoError(t, err)

		assert.Equal(t, email, dbUsername)
		assert.Equal(t, firstName, dbFirstName)
		assert.Equal(t, lastName, dbLastName)
		assert.Equal(t, []string{"role2"}, dbRoles)
	})
}

func Test_execAddUserFunc(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	t.Run("User must be valid", func(t *testing.T) {
		email, password, firstName, lastName := "test@email.com", "mypassword12", "First", "Last"

		// Invalid invalid
		err := execAddUser(ctx, dbt.DSN, "", firstName, lastName, password, false, []string{})
		assert.EqualError(t, err, "error creating user: error creating user: error validating user fields: email is required")

		err = execAddUser(ctx, dbt.DSN, "wrongemail", firstName, lastName, password, false, []string{})
		assert.EqualError(t, err, `error creating user: error creating user: error validating user fields: email is invalid: the provided email "wrongemail" is not valid`)

		// Invalid password
		err = execAddUser(ctx, dbt.DSN, email, firstName, lastName, "pass", false, []string{})
		assert.EqualError(t, err, fmt.Sprintf("error creating user: error creating user: error encrypting password: password should have at least %d characters", auth.MinPasswordLength))

		// Invalid first name
		err = execAddUser(ctx, dbt.DSN, email, "", lastName, "pass", false, []string{})
		assert.EqualError(t, err, "error creating user: error creating user: error validating user fields: first name is required")

		// Invalid last name
		err = execAddUser(ctx, dbt.DSN, email, firstName, "", "pass", false, []string{})
		assert.EqualError(t, err, "error creating user: error creating user: error validating user fields: last name is required")

		// Valid user
		err = execAddUser(ctx, dbt.DSN, email, firstName, lastName, password, false, []string{})
		require.NoError(t, err)
	})

	t.Run("Inserted user must have his password encrypted", func(t *testing.T) {
		email, password, firstName, lastName := "test2@email.com", "mypassword12", "First", "Last"

		err := execAddUser(ctx, dbt.DSN, email, firstName, lastName, password, false, []string{})
		require.NoError(t, err)

		var dbPassword string
		err = dbConnectionPool.QueryRowxContext(ctx, "SELECT encrypted_password FROM auth_users WHERE email = $1", email).Scan(&dbPassword)
		require.NoError(t, err)
		assert.NotEqual(t, password, dbPassword)

		encrypter := auth.NewDefaultPasswordEncrypter()

		compare, err := encrypter.ComparePassword(ctx, dbPassword, password)
		require.NoError(t, err)
		assert.True(t, compare)
	})

	t.Run("Email should be unique", func(t *testing.T) {
		email, password, firstName, lastName := "unique@email.com", "mypassword12", "First", "Last"

		err := execAddUser(ctx, dbt.DSN, email, firstName, lastName, password, false, []string{})
		require.NoError(t, err)

		err = execAddUser(ctx, dbt.DSN, email, firstName, lastName, password, false, []string{})
		assert.EqualError(t, err, `error creating user: error creating user: a user with this email already exists`)
	})

	t.Run("set the user roles", func(t *testing.T) {
		email, password, firstName, lastName := "testroles@email.com", "mypassword12", "First", "Last"

		err := execAddUser(ctx, dbt.DSN, email, firstName, lastName, password, false, []string{"role1", "role2"})
		require.NoError(t, err)

		var dbRoles []string
		err = dbConnectionPool.QueryRowxContext(ctx, "SELECT roles FROM auth_users WHERE email = $1", email).Scan(pq.Array(dbRoles))
		require.NoError(t, err)
		assert.NotEqual(t, []string{"role1", "role2"}, dbRoles)
	})
}

func Test_validateRoles(t *testing.T) {
	err := validateRoles([]string{"role1", "role2"}, []string{"role2", "role3"})
	assert.EqualError(t, err, "invalid role provided. Expected one of these values: role1 | role2")

	err = validateRoles([]string{"role1", "role2"}, []string{"role2", "role1"})
	assert.Nil(t, err)

	err = validateRoles([]string{}, []string{})
	assert.Nil(t, err)
}
