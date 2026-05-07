package utils

import (
	"net/http"
	"testing"
)

func TestHasContentType(t *testing.T) {
	type tc struct {
		name     string
		r        *http.Request
		expected string
		want     bool
	}

	tests := []tc{
		{
			name:     "no Content-Type header",
			r:        &http.Request{Header: http.Header{}},
			expected: "application/json",
			want:     false,
		},
		{
			name:     "empty Content-Type header value",
			r:        &http.Request{Header: http.Header{"Content-Type": []string{""}}},
			expected: "application/json",
			want:     false,
		},
		{
			name:     "invalid Content-Type value (parse error)",
			r:        &http.Request{Header: http.Header{"Content-Type": []string{"not a real content type"}}},
			expected: "application/json",
			want:     false,
		},
		{
			name:     "exact match without parameters",
			r:        &http.Request{Header: http.Header{"Content-Type": []string{"application/json"}}},
			expected: "application/json",
			want:     true,
		},
		{
			name:     "mismatch without parameters",
			r:        &http.Request{Header: http.Header{"Content-Type": []string{"application/xml"}}},
			expected: "application/json",
			want:     false,
		},
		{
			name:     "match with parameters (charset)",
			r:        &http.Request{Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}},
			expected: "application/json",
			want:     true,
		},
		{
			name:     "case-insensitive parsing: header value in different case should match",
			r:        &http.Request{Header: http.Header{"Content-Type": []string{"Multipart/Form-Data; boundary=xyz"}}},
			expected: "multipart/form-data",
			want:     true,
		},
		{
			name:     "expected type must match exactly (no wildcard support)",
			r:        &http.Request{Header: http.Header{"Content-Type": []string{"multipart/form-data; boundary=xyz"}}},
			expected: "multipart/*",
			want:     false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HasContentType(tt.r, tt.expected)
			if got != tt.want {
				t.Fatalf("HasContentType() = %v, want %v (expected=%q, ct=%q)",
					got, tt.want, tt.expected, contentTypeForDebug(tt.r))
			}
		})
	}
}

func TestIsMultipartFormData(t *testing.T) {
	t.Parallel()

	type tc struct {
		name string
		r    *http.Request
		want bool
	}

	tests := []tc{
		{
			name: "nil request",
			r:    nil,
			want: false,
		},
		{
			name: "no Content-Type",
			r:    &http.Request{Header: http.Header{}},
			want: false,
		},
		{
			name: "multipart/form-data with boundary",
			r:    &http.Request{Header: http.Header{"Content-Type": []string{"multipart/form-data; boundary=abc"}}},
			want: true,
		},
		{
			name: "not multipart/form-data",
			r:    &http.Request{Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}},
			want: false,
		},
		{
			name: "invalid Content-Type",
			r:    &http.Request{Header: http.Header{"Content-Type": []string{"bad"}}},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsMultipartFormData(tt.r)
			if got != tt.want {
				t.Fatalf("IsMultipartFormData() = %v, want %v (ct=%q)",
					got, tt.want, contentTypeForDebug(tt.r))
			}
		})
	}
}

// contentTypeForDebug is only for nicer failure messages in tests.
func contentTypeForDebug(r *http.Request) string {
	if r == nil {
		return "<nil request>"
	}
	return r.Header.Get("Content-Type")
}
