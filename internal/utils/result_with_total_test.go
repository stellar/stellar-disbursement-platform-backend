package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ResultWithTotal_NewResultWithTotal(t *testing.T) {
	total := 10
	result := []string{"apple", "banana", "cherry"}

	resultWithTotal := NewResultWithTotal(total, result)

	require.Equal(t, total, resultWithTotal.Total)
	require.Equal(t, result, resultWithTotal.Result)
}
