package monitor

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_sanitizeHTTPMethod(t *testing.T) {
	testCases := []struct {
		name   string
		method string
		want   string
	}{
		{name: "GET passes through", method: http.MethodGet, want: "GET"},
		{name: "HEAD passes through", method: http.MethodHead, want: "HEAD"},
		{name: "POST passes through", method: http.MethodPost, want: "POST"},
		{name: "PUT passes through", method: http.MethodPut, want: "PUT"},
		{name: "PATCH passes through", method: http.MethodPatch, want: "PATCH"},
		{name: "DELETE passes through", method: http.MethodDelete, want: "DELETE"},
		{name: "CONNECT passes through", method: http.MethodConnect, want: "CONNECT"},
		{name: "OPTIONS passes through", method: http.MethodOptions, want: "OPTIONS"},
		{name: "TRACE passes through", method: http.MethodTrace, want: "TRACE"},
		{name: "lower case collapses", method: "get", want: unknownMethod},
		{name: "mixed case collapses", method: "PoSt", want: unknownMethod},
		{name: "unknown token collapses", method: "ZZBOGUS", want: unknownMethod},
		{name: "arbitrary attacker token collapses", method: "XM00000001", want: unknownMethod},
		{name: "empty method collapses", method: "", want: unknownMethod},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitizeHTTPMethod(tc.method))
		})
	}
}
