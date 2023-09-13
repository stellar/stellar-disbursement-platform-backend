package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

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
	return reflect.ValueOf(&v).Elem().IsZero()
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
