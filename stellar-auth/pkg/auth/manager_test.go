package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_User_Validate(t *testing.T) {
	user := &User{
		ID:        "",
		FirstName: "",
		LastName:  "",
		Email:     "",
		IsOwner:   false,
		Roles:     []string{},
	}

	assert.EqualError(t, user.Validate(), "email is required")

	user.Email = "invalid"
	assert.EqualError(t, user.Validate(), `email is invalid: the provided email "invalid" is not valid`)

	user.Email = "email@email.com"
	assert.EqualError(t, user.Validate(), "first name is required")

	user.FirstName = "First"
	assert.EqualError(t, user.Validate(), "last name is required")

	user.LastName = "Last"
	assert.NoError(t, user.Validate())
}
