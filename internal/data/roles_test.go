package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_UserRole_IsValid(t *testing.T) {
	role := UserRole("unknown")
	assert.False(t, role.IsValid())

	role = UserRole("developer")
	assert.True(t, role.IsValid())
}
