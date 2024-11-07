package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func GetRoutePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if pattern := rctx.RoutePattern(); pattern != "" {
		// Pattern is already available
		return pattern
	}

	routePath := r.URL.Path

	if r.URL.RawPath != "" {
		routePath = r.URL.RawPath
	}

	tctx := chi.NewRouteContext()
	if !rctx.Routes.Match(tctx, r.Method, routePath) {
		return "undefined"
	}

	// tctx has the updated pattern, since Match mutates it
	return tctx.RoutePattern()
}

// UnwrapInterfaceToPointer unwraps an interface to a pointer of the given type.
func UnwrapInterfaceToPointer[T any](i interface{}) *T {
	t, ok := i.(*T)
	if ok {
		return t
	}
	return nil
}

// IsEmpty checks if a value is empty.
func IsEmpty[T any](v T) bool {
	valueType := reflect.TypeOf(v)
	if valueType == nil { // this condition will be true when v is nil and valueType is either `any` or `interface{}`
		return true
	}

	return reflect.DeepEqual(v, reflect.Zero(valueType).Interface())
}

func MapSlice[T any, M any](a []T, f func(T) M) []M {
	n := make([]M, len(a))
	for i, e := range a {
		n[i] = f(e)
	}
	return n
}

func ConvertType[S any, D any](src S) (D, error) {
	jsonBody, err := json.Marshal(src)
	if err != nil {
		return *new(D), fmt.Errorf("converting source into json: %w", err)
	}

	var dst D
	err = json.Unmarshal(jsonBody, &dst)
	if err != nil {
		return *new(D), fmt.Errorf("converting json into destination: %w", err)
	}

	return dst, nil
}

// GetTypeName receives any value and returns the name of its type without the package prefix.
func GetTypeName(v interface{}) string {
	if v == nil {
		return "<nil>"
	}

	fullTypeName := fmt.Sprintf("%T", v)

	if dotIndex := strings.LastIndex(fullTypeName, "."); dotIndex != -1 {
		return fullTypeName[dotIndex+1:]
	}

	return fullTypeName
}

// StringPtr returns a pointer to a string
func StringPtr(s string) *string {
	return &s
}

// IntPtr returns a pointer to an int
func IntPtr(i int) *int {
	return &i
}

func TimePtr(t time.Time) *time.Time {
	return &t
}

func VisualBool(b bool) string {
	if b {
		return "🟢"
	}
	return "🔴"
}

// ParseBoolQueryParam parses a boolean query parameter from an HTTP request.
func ParseBoolQueryParam(r *http.Request, param string) (*bool, error) {
	paramValue := r.URL.Query().Get(param)
	if paramValue == "" {
		return nil, nil
	}
	parsedValue, err := strconv.ParseBool(paramValue)
	if err != nil {
		return nil, fmt.Errorf("invalid '%s' parameter value: %w", param, err)
	}
	return &parsedValue, nil
}
