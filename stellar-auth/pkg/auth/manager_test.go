package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_User_Validate(t *testing.T) {
	testCases := []struct {
		name      string
		user      *User
		wantEmail string
		wantFirst string
		wantLast  string
	}{
		{
			name: "trims whitespace and lowercases email",
			user: &User{
				Email:     "  Test@Email.com  ",
				FirstName: "First",
				LastName:  "Last",
			},
			wantEmail: "test@email.com",
			wantFirst: "First",
			wantLast:  "Last",
		},
		{
			name: "trims whitespace from names",
			user: &User{
				Email:     "test@email.com",
				FirstName: "  First  ",
				LastName:  "  Last  ",
			},
			wantEmail: "test@email.com",
			wantFirst: "First",
			wantLast:  "Last",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.user.SanitizeAndValidate()
			assert.NoError(t, err)
			assert.Equal(t, tc.wantEmail, tc.user.Email)
			assert.Equal(t, tc.wantFirst, tc.user.FirstName)
			assert.Equal(t, tc.wantLast, tc.user.LastName)
		})
	}
}

func Test_User_SanitizeAndValidate_Validation(t *testing.T) {
	testCases := []struct {
		name    string
		user    *User
		wantErr string
	}{
		{
			name:    "empty email returns error",
			user:    &User{},
			wantErr: "email is required",
		},
		{
			name: "invalid email returns error",
			user: &User{
				Email: "invalid",
			},
			wantErr: `email is invalid: the provided email "invalid" is not valid`,
		},
		{
			name: "empty first name returns error",
			user: &User{
				Email: "test@email.com",
			},
			wantErr: "first name is required",
		},
		{
			name: "empty last name returns error",
			user: &User{
				Email:     "test@email.com",
				FirstName: "First",
			},
			wantErr: "last name is required",
		},
		{
			name: "valid user returns no error",
			user: &User{
				Email:     "test@email.com",
				FirstName: "First",
				LastName:  "Last",
			},
			wantErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.user.SanitizeAndValidate()
			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
