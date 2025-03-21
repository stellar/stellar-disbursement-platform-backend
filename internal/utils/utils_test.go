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
		// Any:
		{name: "Any nil", isEmptyFn: func() bool { return IsEmpty[any](nil) }, expected: true},
		{name: "Any non-nil", isEmptyFn: func() bool { return IsEmpty[any](new(string)) }, expected: false},
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

func Test_ConvertType(t *testing.T) {
	t.Run("converts a struct to another struct", func(t *testing.T) {
		type srcStruct struct {
			Name string
			Foo  string
		}
		type dstStruct struct {
			Name string
			Bar  string
		}

		src := srcStruct{Name: "test"}
		wantDst := dstStruct{Name: "test"}
		dst, err := ConvertType[srcStruct, dstStruct](src)
		require.NoError(t, err)
		assert.Equal(t, wantDst, dst)
	})

	t.Run("converts int into float", func(t *testing.T) {
		src := 1
		wantDst := float32(1)
		dst, err := ConvertType[int, float32](src)
		require.NoError(t, err)
		assert.Equal(t, wantDst, dst)
	})
}

func Test_GetTypeName(t *testing.T) {
	type MyType struct{}

	testCases := []struct {
		name           string
		instance       interface{}
		expectedResult string
	}{
		{
			name:           "nil",
			instance:       nil,
			expectedResult: "<nil>",
		},
		{
			name:           "Integer",
			instance:       42,
			expectedResult: "int",
		},
		{
			name:           "Pointer to int",
			instance:       new(int),
			expectedResult: "*int",
		},
		{
			name:           "String",
			instance:       "test",
			expectedResult: "string",
		},
		{
			name:           "Pointer to string",
			instance:       new(string),
			expectedResult: "*string",
		},
		{
			name:           "Empty struct",
			instance:       struct{}{},
			expectedResult: "struct {}",
		},
		{
			name:           "Slice of strings",
			instance:       []string{},
			expectedResult: "[]string",
		},
		{
			name:           "Map",
			instance:       map[string]int{},
			expectedResult: "map[string]int",
		},
		{
			name:           "Custom type",
			instance:       MyType{},
			expectedResult: "MyType",
		},
		{
			name:           "Pointer to custom type",
			instance:       new(MyType),
			expectedResult: "MyType",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualResult := GetTypeName(tc.instance)
			assert.Equal(t, tc.expectedResult, actualResult)
		})
	}
}

func Test_StringPtr(t *testing.T) {
	t.Run("returns a pointer to the string", func(t *testing.T) {
		s := "test string"
		result := StringPtr(s)

		assert.NotNil(t, result)
		assert.Equal(t, s, *result)
	})

	t.Run("returns a pointer to an empty string", func(t *testing.T) {
		s := ""
		result := StringPtr(s)

		assert.NotNil(t, result)
		assert.Equal(t, s, *result)
	})

	t.Run("changing the original string does not affect the pointer", func(t *testing.T) {
		s := "initial string"
		result := StringPtr(s)

		// Modify the original string
		s = "modified string"

		assert.NotNil(t, result)
		assert.NotEqual(t, s, *result)
		assert.Equal(t, "initial string", *result)
	})
}

// Write a test for ParseBoolQueryParam function.
func Test_ParseBoolQueryParam(t *testing.T) {
	trueValue := true
	falseValue := false

	testCases := []struct {
		name           string
		queryParam     string
		expectedResult *bool
		expectedError  string
	}{
		{
			name:           "valid true value",
			queryParam:     "true",
			expectedResult: &trueValue,
			expectedError:  "",
		},
		{
			name:           "valid false value",
			queryParam:     "false",
			expectedResult: &falseValue,
			expectedError:  "",
		},
		{
			name:           "valid empty value",
			queryParam:     "",
			expectedResult: nil,
			expectedError:  "",
		},
		{
			name:           "invalid value",
			queryParam:     "invalid",
			expectedResult: nil,
			expectedError:  "invalid 'enabled' parameter value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", fmt.Sprintf("/?enabled=%s", tc.queryParam), nil)
			require.NoError(t, err)

			result, err := ParseBoolQueryParam(req, "enabled")
			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}
