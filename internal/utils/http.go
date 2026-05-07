package utils

import (
	"mime"
	"net/http"
)

// HasContentType reports whether the request's Content-Type matches
// the provided media type (e.g. "application/json", "multipart/form-data").
//
// It parses the header using mime.ParseMediaType so it correctly handles
// parameters like boundary and charset and is RFC-compliant.
func HasContentType(r *http.Request, expected string) bool {
	if r == nil {
		return false
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return mediaType == expected
}

// IsMultipartFormData reports whether the request's Content-Type
// is "multipart/form-data".
func IsMultipartFormData(r *http.Request) bool {
	return HasContentType(r, "multipart/form-data")
}
