package httpresponse

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// PaginatedResponse is a response that contains pagination information.
type PaginatedResponse struct {
	Pagination PaginationInfo  `json:"pagination"`
	Data       json.RawMessage `json:"data"`
}

type PaginationInfo struct {
	Next  string `json:"next,omitempty"`
	Prev  string `json:"prev,omitempty"`
	Pages int    `json:"pages"`
	Total int    `json:"total"`
}

// NewEmptyPaginatedResponse returns a PaginatedResponse with an empty data and 0 pages.
//
//	This is useful for returning an empty list.
func NewEmptyPaginatedResponse() PaginatedResponse {
	return PaginatedResponse{
		Pagination: PaginationInfo{
			Pages: 0,
			Total: 0,
		},
		Data: json.RawMessage("[]"),
	}
}

// NewPaginatedResponse returns a PaginatedResponse with pagination information.
func NewPaginatedResponse(r *http.Request, data interface{}, currentPage, pageLimit, totalItems int) (PaginatedResponse, error) {
	totalPages := (totalItems + pageLimit - 1) / pageLimit
	pagination := PaginationInfo{Pages: totalPages, Total: totalItems}

	baseURL := *r.URL
	q := baseURL.Query()
	q.Del("page")

	if currentPage < totalPages {
		q.Set("page", fmt.Sprintf("%d", currentPage+1))
		baseURL.RawQuery = q.Encode()
		pagination.Next = baseURL.String()
	}

	if currentPage > 1 {
		q.Set("page", fmt.Sprintf("%d", currentPage-1))
		baseURL.RawQuery = q.Encode()
		pagination.Prev = baseURL.String()
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return PaginatedResponse{}, err
	}

	return PaginatedResponse{
		Pagination: pagination,
		Data:       dataBytes,
	}, nil
}
