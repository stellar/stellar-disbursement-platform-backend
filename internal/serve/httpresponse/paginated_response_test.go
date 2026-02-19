package httpresponse

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewPaginatedResponse_ErrorsOnInvalidPageLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/payments?page=1&page_limit=0", nil)
	_, err := NewPaginatedResponse(req, []string{"a"}, 1, 0, 1)
	assert.ErrorContains(t, err, "page_limit must be a positive integer")
}

func Test_NewPaginatedResponse_BuildsPaginationLinks(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/payments?page=1&page_limit=2", nil)
	resp, err := NewPaginatedResponse(req, []string{"a", "b"}, 1, 2, 5)
	assert.NoError(t, err)
	assert.Equal(t, 3, resp.Pagination.Pages)
	assert.Equal(t, 5, resp.Pagination.Total)
	assert.Equal(t, "http://example.com/payments?page=2&page_limit=2", resp.Pagination.Next)
	assert.Empty(t, resp.Pagination.Prev)
}
