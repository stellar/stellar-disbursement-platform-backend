package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetRoutePattern(t *testing.T) {
	testCases := []struct {
		expectedRoutePattern string
		method               string
	}{
		{expectedRoutePattern: "/mock", method: "GET"},
		{expectedRoutePattern: "undefined", method: "POST"},
	}

	mHttpHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, tc := range testCases {
		t.Run("getting route pattern", func(t *testing.T) {
			mAssertRoutePattern := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
					routePattern := GetRoutePattern(req)

					assert.Equal(t, tc.expectedRoutePattern, routePattern)
					next.ServeHTTP(rw, req)
				})
			}

			r := chi.NewRouter()
			r.Use(mAssertRoutePattern)
			r.Get("/mock", mHttpHandler.ServeHTTP)

			req, err := http.NewRequest(tc.method, "/mock", nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
		})
	}
}

func Test_UnwrapInterfaceToPointer(t *testing.T) {
	// Test with a string
	strValue := "test"
	strValuePtr := &strValue
	i := interface{}(strValuePtr)

	unwrappedValue := UnwrapInterfaceToPointer[string](i)
	assert.Equal(t, "test", *unwrappedValue)

	// Test with a struct
	type testStruct struct {
		Name string
	}
	testStructValue := testStruct{Name: "test"}
	testStructValuePtr := &testStructValue
	i = interface{}(testStructValuePtr)
	assert.Equal(t, testStruct{Name: "test"}, *UnwrapInterfaceToPointer[testStruct](i))
}

func Test_IsEmpty(t *testing.T) {
	type testCase struct {
		name      string
		isEmptyFn func() bool
		expected  bool
	}

	// testStruct is used just for testing empty and non empty structs.
	type testStruct struct{ Name string }

	// Define test cases
	testCases := []testCase{
		// String
		{name: "String empty", isEmptyFn: func() bool { return IsEmpty[string]("") }, expected: true},
		{name: "String non-empty", isEmptyFn: func() bool { return IsEmpty[string]("not empty") }, expected: false},
		// Int
		{name: "Int zero", isEmptyFn: func() bool { return IsEmpty[int](0) }, expected: true},
		{name: "Int non-zero", isEmptyFn: func() bool { return IsEmpty[int](1) }, expected: false},
		// Slice:
		{name: "Slice nil", isEmptyFn: func() bool { return IsEmpty[[]string](nil) }, expected: true},
		{name: "Slice empty", isEmptyFn: func() bool { return IsEmpty[[]string]([]string{}) }, expected: false},
		{name: "Slice non-empty", isEmptyFn: func() bool { return IsEmpty[[]string]([]string{"not empty"}) }, expected: false},
		// Struct:
		{name: "Struct zero", isEmptyFn: func() bool { return IsEmpty[testStruct](testStruct{}) }, expected: true},
		{name: "Struct non-zero", isEmptyFn: func() bool { return IsEmpty[testStruct](testStruct{Name: "not empty"}) }, expected: false},
		// Pointer:
		{name: "Pointer nil", isEmptyFn: func() bool { return IsEmpty[*string](nil) }, expected: true},
		{name: "Pointer non-nil", isEmptyFn: func() bool { return IsEmpty[*string](new(string)) }, expected: false},
		// Function:
		{name: "Function nil", isEmptyFn: func() bool { return IsEmpty[func() string](nil) }, expected: true},
		{name: "Function non-nil", isEmptyFn: func() bool { return IsEmpty[func() string](func() string { return "not empty" }) }, expected: false},
		// Interface:
		{name: "Interface nil", isEmptyFn: func() bool { return IsEmpty[interface{}](nil) }, expected: true},
		{name: "Interface non-nil", isEmptyFn: func() bool { return IsEmpty[interface{}](new(string)) }, expected: false},
		// Map:
		{name: "Map nil", isEmptyFn: func() bool { return IsEmpty[map[string]string](nil) }, expected: true},
		{name: "Map empty", isEmptyFn: func() bool { return IsEmpty[map[string]string](map[string]string{}) }, expected: false},
		{name: "Map non-empty", isEmptyFn: func() bool { return IsEmpty[map[string]string](map[string]string{"not empty": "not empty"}) }, expected: false},
		// Channel:
		{name: "Channel nil", isEmptyFn: func() bool { return IsEmpty[chan string](nil) }, expected: true},
		{name: "Channel non-nil", isEmptyFn: func() bool { return IsEmpty[chan string](make(chan string)) }, expected: false},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.isEmptyFn())
		})
	}
}

func Test_MapSlice(t *testing.T) {
	testCases := []struct {
		name              string
		prepareMapSliceFn func() interface{}
		wantMapped        interface{}
	}{
		{
			name: "map to string slice to uppercased string slice",
			prepareMapSliceFn: func() interface{} {
				return MapSlice([]string{"a", "b", "c"}, strings.ToUpper)
			},
			wantMapped: []string{"A", "B", "C"},
		},
		{
			name: "map int slice to string slice",
			prepareMapSliceFn: func() interface{} {
				return MapSlice([]int{1, 2, 3}, func(input int) string { return fmt.Sprintf("%d", input) })
			},
			wantMapped: []string{"1", "2", "3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotMapped := tc.prepareMapSliceFn()
			require.Equal(t, tc.wantMapped, gotMapped)
		})
	}
}
