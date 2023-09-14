package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
		{"", fmt.Errorf("email cannot be empty")},
		{"notvalidemail", fmt.Errorf("the provided email is not valid")},
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
				assert.EqualErrorf(t, gotError, tc.wantErr.Error(), "ValidateURL(%q) should be '%v', but got '%v'", tc.url, tc.wantErr, gotError)
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
