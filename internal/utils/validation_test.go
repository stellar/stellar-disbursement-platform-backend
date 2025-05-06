package utils

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

func Test_ValidatePhoneNumber(t *testing.T) {
	testCases := []struct {
		phoneNumber string
		wantErr     error
	}{
		{"", ErrEmptyPhoneNumber},
		{"notvalidphone", ErrInvalidE164PhoneNumber},
		{"14155555555", ErrInvalidE164PhoneNumber},
		{"+380445555555", nil},
		{"+14155555555x4444", ErrInvalidE164PhoneNumber},
		{"+1 415 555 5555", ErrInvalidE164PhoneNumber},
		{"+1 415-555-5555", ErrInvalidE164PhoneNumber},
		{"+05555555555", ErrInvalidE164PhoneNumber},
		{"++5555555555", ErrInvalidE164PhoneNumber},
		{"+38012345678", ErrInvalidE164PhoneNumber},
		{"+38056789013", ErrInvalidE164PhoneNumber},
		{"+38034567890", ErrInvalidE164PhoneNumber},
		{"+15555555555", ErrInvalidE164PhoneNumber},
		{"+14155555555", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.phoneNumber, func(t *testing.T) {
			gotError := ValidatePhoneNumber(tc.phoneNumber)
			assert.Equalf(t, tc.wantErr, gotError, "ValidatePhoneNumber(%q) should be %v, but got %v", tc.phoneNumber, tc.wantErr, gotError)
		})
	}
}

func Test_ValidatePathIsNotTraversal(t *testing.T) {
	testCases := []struct {
		path        string
		isTraversal bool
	}{
		{"", false},
		{"http://example.com", false},
		{"documents", false},
		{"./documents/files", false},
		{"./projects/subproject/report", false},
		{"http://example.com/../config.yaml", true},
		{"../config.yaml", true},
		{"documents/../config.yaml", true},
		{"docs/files/..", true},
		{"..\\config.yaml", true},
		{"documents\\..\\config.yaml", true},
		{"documents\\files\\..", true},
	}

	for _, tc := range testCases {
		t.Run("-"+tc.path, func(t *testing.T) {
			err := ValidatePathIsNotTraversal(tc.path)
			if tc.isTraversal {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ValidateAmount(t *testing.T) {
	testCases := []struct {
		amount  string
		wantErr error
	}{
		{"", fmt.Errorf("amount cannot be empty")},
		{"notvalidamount", fmt.Errorf("the provided amount is not a valid number")},
		{"0", fmt.Errorf("the provided amount must be greater than zero")},
		{"0.00", fmt.Errorf("the provided amount must be greater than zero")},
		{"1", nil},
		{"1.00", nil},
		{"1.01", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.amount, func(t *testing.T) {
			gotError := ValidateAmount(tc.amount)
			assert.Equalf(t, tc.wantErr, gotError, "ValidateAmount(%q) should be %v, but got %v", tc.amount, tc.wantErr, gotError)
		})
	}
}

func Test_ValidateEmail(t *testing.T) {
	testCases := []struct {
		email   string
		wantErr error
	}{
		{"", fmt.Errorf("email field is required")},
		{"notvalidemail", fmt.Errorf("the email address provided is not valid")},
		{"valid@test.com", nil},
		{"valid+email@test.com", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.email, func(t *testing.T) {
			gotError := ValidateEmail(tc.email)
			assert.Equalf(t, tc.wantErr, gotError, "ValidateEmail(%q) should be %v, but got %v", tc.email, tc.wantErr, gotError)
		})
	}
}

func TestValidateStringLength(t *testing.T) {
	tests := []struct {
		name        string
		field       string
		fieldName   string
		maxLength   int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "🔴error - empty field",
			field:       "",
			fieldName:   "username",
			maxLength:   50,
			expectError: true,
			errorMsg:    "username field is required",
		},
		{
			name:        "🔴error - field with only spaces",
			field:       "   ",
			fieldName:   "username",
			maxLength:   50,
			expectError: true,
			errorMsg:    "username field is required",
		},
		{
			name:        "🔴error - field exceeds max length",
			field:       strings.Repeat("a", 51),
			fieldName:   "username",
			maxLength:   50,
			expectError: true,
			errorMsg:    "username cannot exceed 50 characters",
		},
		{
			name:        "🔴error - field with spaces exceeds max length",
			field:       "  " + strings.Repeat("a", 49) + "  ",
			fieldName:   "username",
			maxLength:   50,
			expectError: true,
			errorMsg:    "username cannot exceed 50 characters",
		},
		{
			name:        "🟢success - field at exact max length",
			field:       strings.Repeat("a", 50),
			fieldName:   "username",
			maxLength:   50,
			expectError: false,
		},
		{
			name:        "🟢success - field under max length",
			field:       "John Doe",
			fieldName:   "username",
			maxLength:   50,
			expectError: false,
		},
		{
			name:        "🟢success - field with leading/trailing spaces but still under max length",
			field:       "  John Doe  ",
			fieldName:   "username",
			maxLength:   50,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStringLength(tc.field, tc.fieldName, tc.maxLength)
			if tc.expectError {
				assert.Error(t, err)
				assert.Equal(t, tc.errorMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ValidateDNS(t *testing.T) {
	testCases := []struct {
		url     string
		wantErr error
	}{
		{"localhost", nil},
		{"a.bc", nil},
		{"test.com", nil},
		{"a.b..", fmt.Errorf(`"a.b.." is not a valid DNS name`)},
		{"localhost.local", nil},
		{"localhost.localdomain.intern", nil},
		{"l.local.intern", nil},
		{"ru.link.n.svpncloud.com", nil},
		{"-localhost", fmt.Errorf(`"-localhost" is not a valid DNS name`)},
		{"localhost.-localdomain", fmt.Errorf(`"localhost.-localdomain" is not a valid DNS name`)},
		{"localhost.localdomain.-int", fmt.Errorf(`"localhost.localdomain.-int" is not a valid DNS name`)},
		{"localhost._localdomain", nil},
		{"localhost.localdomain._int", nil},
		{"lÖcalhost", fmt.Errorf(`"lÖcalhost" is not a valid DNS name`)},
		{"localhost.lÖcaldomain", fmt.Errorf(`"localhost.lÖcaldomain" is not a valid DNS name`)},
		{"localhost.localdomain.üntern", fmt.Errorf(`"localhost.localdomain.üntern" is not a valid DNS name`)},
		{"localhost/", fmt.Errorf(`"localhost/" is not a valid DNS name`)},
		{"127.0.0.1", fmt.Errorf(`"127.0.0.1" is not a valid DNS name`)},
		{"[::1]", fmt.Errorf(`"[::1]" is not a valid DNS name`)},
		{"50.50.50.50", fmt.Errorf(`"50.50.50.50" is not a valid DNS name`)},
		{"localhost.localdomain.intern:65535", fmt.Errorf(`"localhost.localdomain.intern:65535" is not a valid DNS name`)},
		{"漢字汉字", fmt.Errorf(`"漢字汉字" is not a valid DNS name`)},
		{"www.jubfvq1v3p38i51622y0dvmdk1mymowjyeu26gbtw9andgynj1gg8z3msb1kl5z6906k846pj3sulm4kiyk82ln5teqj9nsht59opr0cs5ssltx78lfyvml19lfq1wp4usbl0o36cmiykch1vywbttcus1p9yu0669h8fj4ll7a6bmop505908s1m83q2ec2qr9nbvql2589adma3xsq2o38os2z3dmfh2tth4is4ixyfasasasefqwe4t2ub2fz1rme.de", fmt.Errorf(`"www.jubfvq1v3p38i51622y0dvmdk1mymowjyeu26gbtw9andgynj1gg8z3msb1kl5z6906k846pj3sulm4kiyk82ln5teqj9nsht59opr0cs5ssltx78lfyvml19lfq1wp4usbl0o36cmiykch1vywbttcus1p9yu0669h8fj4ll7a6bmop505908s1m83q2ec2qr9nbvql2589adma3xsq2o38os2z3dmfh2tth4is4ixyfasasasefqwe4t2ub2fz1rme.de" is not a valid DNS name`)},
	}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			gotError := ValidateDNS(tc.url)

			if tc.wantErr != nil {
				assert.EqualErrorf(t, gotError, tc.wantErr.Error(), "ValidateDNS(%q) should be '%v', but got '%v'", tc.url, tc.wantErr, gotError)
			} else {
				assert.NoError(t, gotError)
			}
		})
	}
}

func Test_ValidateOTP(t *testing.T) {
	testCases := []struct {
		otp     string
		wantErr error
	}{
		{"", fmt.Errorf("otp cannot be empty")},
		{"mock", fmt.Errorf("the provided OTP is not a valid 6 digits value")},
		{"123", fmt.Errorf("the provided OTP is not a valid 6 digits value")},
		{"12mock", fmt.Errorf("the provided OTP is not a valid 6 digits value")},
		{"123456", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.otp, func(t *testing.T) {
			gotError := ValidateOTP(tc.otp)
			assert.Equalf(t, tc.wantErr, gotError, "ValidateOTP(%q) should be %v, but got %v", tc.otp, tc.wantErr, gotError)
		})
	}
}

func Test_ValidateDateOfBirthVerification(t *testing.T) {
	tests := []struct {
		name              string
		dob               string
		expectedError     error
		expectedErrorCode string
	}{
		{"valid DOB", "1990-01-30", nil, ""},
		{"invalid DOB - empty DOB", "", fmt.Errorf("date of birth cannot be empty"), httperror.Extra_0},
		{"invalid DOB - invalid format", "30-01-1990", fmt.Errorf("invalid date of birth format. Correct format: 1990-01-30"), httperror.Extra_2},
		{"invalid DOB - future date", time.Now().AddDate(1, 0, 0).Format("2006-01-02"), fmt.Errorf("date of birth cannot be in the future"), httperror.Extra_4},
		{"invalid DOB - invalid day", "1990-01-32", fmt.Errorf("invalid date of birth format. Correct format: 1990-01-30"), httperror.Extra_2},
		{"invalid DOB - invalid month", "1990-13-01", fmt.Errorf("invalid date of birth format. Correct format: 1990-01-30"), httperror.Extra_2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := ValidateDateOfBirthVerification(tt.dob)
			assert.Equal(t, tt.expectedErrorCode, code)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func Test_ValidateYearMonthVerification(t *testing.T) {
	tests := []struct {
		name              string
		yearMonth         string
		expectedError     error
		expectedErrorCode string
	}{
		{"valid yearMonth", "1990-12", nil, ""},
		{"invalid yearMonth - yearMonth DOB", "", fmt.Errorf("year/month cannot be empty"), httperror.Extra_0},
		{"invalid yearMonth - invalid format", "01-1990", fmt.Errorf("invalid year/month format. Correct format: 1990-12"), httperror.Extra_3},
		{"invalid yearMonth - future date", time.Now().AddDate(1, 0, 0).Format("2006-01"), fmt.Errorf("year/month cannot be in the future"), httperror.Extra_4},
		{"invalid yearMonth - invalid month", "1990-13", fmt.Errorf("invalid year/month format. Correct format: 1990-12"), httperror.Extra_3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := ValidateYearMonthVerification(tt.yearMonth)
			assert.Equal(t, tt.expectedErrorCode, code)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func Test_ValidatePinVerification(t *testing.T) {
	tests := []struct {
		name              string
		pin               string
		expectedError     error
		expectedErrorCode string
	}{
		{"valid PIN", "1234", nil, ""},
		{"invalid PIN - too short", "123", fmt.Errorf("invalid pin length. Cannot have less than %d or more than %d characters in pin", VerificationFieldPinMinLength, VerificationFieldPinMaxLength), httperror.Extra_5},
		{"invalid PIN - too long", "12345678901", fmt.Errorf("invalid pin length. Cannot have less than %d or more than %d characters in pin", VerificationFieldPinMinLength, VerificationFieldPinMaxLength), httperror.Extra_5},
		{"invalid PIN - empty", "", fmt.Errorf("invalid pin length. Cannot have less than %d or more than %d characters in pin", VerificationFieldPinMinLength, VerificationFieldPinMaxLength), httperror.Extra_5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := ValidatePinVerification(tt.pin)
			assert.Equal(t, tt.expectedErrorCode, code)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func Test_ValidateNationalIDVerification(t *testing.T) {
	tests := []struct {
		name              string
		nationalID        string
		expectedError     error
		expectedErrorCode string
	}{
		{"valid National ID", "1234567890", nil, ""},
		{"invalid National ID - empty", "", fmt.Errorf("national id cannot be empty"), httperror.Extra_0},
		{"invalid National ID - too long", fmt.Sprintf("%0*d", VerificationFieldMaxIdLength+1, 0), fmt.Errorf("invalid national id. Cannot have more than %d characters in national id", VerificationFieldMaxIdLength), httperror.Extra_6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := ValidateNationalIDVerification(tt.nationalID)
			assert.Equal(t, tt.expectedErrorCode, code)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func Test_ValidateURLScheme(t *testing.T) {
	tests := []struct {
		url             string
		wantErrContains string
		schemas         []string
	}{
		{"https://example.com", "", nil},
		{"https://example.com/page.html", "", nil},
		{"https://example.com/section", "", nil},
		{"https://www.example.com", "", nil},
		{"https://subdomain.example.com", "", nil},
		{"https://www.subdomain.example.com", "", nil},
		{"", "invalid URL format", nil},
		{" ", "invalid URL format", nil},
		{"foobar", "invalid URL format", nil},
		{"foobar", "invalid URL format", nil},
		{"https://", "invalid URL format", nil},
		{"example.com", "invalid URL format", []string{"https"}},
		{"ftp://example.com", "invalid URL scheme is not part of [https]", []string{"https"}},
		{"http://example.com", "invalid URL scheme is not part of [https]", []string{"https"}},
		{"ftp://example.com", "", []string{"ftp"}},
		{"http://example.com", "", []string{"http"}},
	}

	for _, tc := range tests {
		title := fmt.Sprintf("%s-%s", VisualBool(tc.wantErrContains == ""), tc.url)
		t.Run(title, func(t *testing.T) {
			err := ValidateURLScheme(tc.url, tc.schemas...)
			if tc.wantErrContains == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErrContains)
			}
		})
	}
}

func Test_ValidateNoHTML(t *testing.T) {
	rawHTMLTestCases := []string{
		"<a href='evil.com'>Click here</a>",
		"<A HREF='evil.com'>Click here</A>",
		"<style>body { background: red; }</style>",
		"<STYLE>body { background: red; }</STYLE>",
		"<div style='color: red;'>Test</div>",
		"<DIV STYLE='color: red;'>Test</DIV>",
		"expression(alert('XSS'))",
		"EXPRESSION(ALERT('XSS'))",
		"javascript:alert(localStorage.getItem('sdp_session'))",
		"JAVASCRIPT:ALERT(localStorage.getItem('sdp_session'))",
		"javascript:alert('XSS')",
		"JAVASCRIPT:ALERT('XSS')",
	}

	for i, tc := range rawHTMLTestCases {
		t.Run(fmt.Sprintf("rawHTML/%d(%s)", i, tc), func(t *testing.T) {
			err := ValidateNoHTML(tc)
			require.Error(t, err, "ValidateNoHTML(%q) didn't catch the error", tc)
		})
	}

	for i, tc := range rawHTMLTestCases {
		encodedHtmlStr := html.EscapeString(tc)
		t.Run(fmt.Sprintf("encodedHTML/%d(%s)", i, encodedHtmlStr), func(t *testing.T) {
			err := ValidateNoHTML(encodedHtmlStr)
			require.Error(t, err, "ValidateNoHTML(%q) didn't catch the error", encodedHtmlStr)
		})
	}
}
