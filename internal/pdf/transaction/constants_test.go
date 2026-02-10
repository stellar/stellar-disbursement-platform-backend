package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetStellarExpertBaseURL(t *testing.T) {
	url := GetStellarExpertBaseURL()
	assert.NotEmpty(t, url, "GetStellarExpertBaseURL should return a non-empty URL")
	assert.Contains(t, url, "stellar.expert", "default or env URL should point to stellar.expert")
	// Default (when STELLAR_EXPERT_URL is unset) ends with slash
	assert.Regexp(t, `/$`, url, "URL should end with trailing slash for consistent concatenation")
}
