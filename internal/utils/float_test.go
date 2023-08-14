package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_FloatToString(t *testing.T) {
	testCases := []struct {
		floatInput       float64
		wantStringOutput string
	}{
		{floatInput: 1.2345678, wantStringOutput: "1.2345678"},
		{floatInput: 1.23456784, wantStringOutput: "1.2345678"},
		{floatInput: 1.23456789, wantStringOutput: "1.2345679"},
		{floatInput: 1.0, wantStringOutput: "1.0000000"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf(tc.wantStringOutput), func(t *testing.T) {
			gotStringOutput := FloatToString(tc.floatInput)
			assert.Equal(t, tc.wantStringOutput, gotStringOutput)
		})
	}
}
