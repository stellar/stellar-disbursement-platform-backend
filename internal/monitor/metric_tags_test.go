package monitor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_MetricTag_ListAll_IncludesDBMetrics(t *testing.T) {
	allTags := MetricTag("").ListAll()

	expectedDBTags := []MetricTag{
		DBOpenConnectionsTag,
		DBInUseConnectionsTag,
		DBIdleConnectionsTag,
		DBMaxOpenConnectionsTag,
		DBWaitCountTotalTag,
		DBWaitDurationSecondsTotalTag,
		DBMaxIdleClosedTotalTag,
		DBMaxIdleTimeClosedTotalTag,
		DBMaxLifetimeClosedTotalTag,
	}

	for _, expectedTag := range expectedDBTags {
		assert.Contains(t, allTags, expectedTag)
	}
}

func Test_MetricTag_ListAll_IncludesExistingMetrics(t *testing.T) {
	allTags := MetricTag("").ListAll()

	// Verify existing metrics are still included
	existingTags := []MetricTag{
		SuccessfulQueryDurationTag,
		FailureQueryDurationTag,
		HTTPRequestDurationTag,
		DisbursementsCounterTag,
		CircleAPIRequestDurationTag,
		CircleAPIRequestsTotalTag,
	}

	for _, existingTag := range existingTags {
		assert.Contains(t, allTags, existingTag)
	}
}

func Test_MetricTag_ListAll_Count(t *testing.T) {
	allTags := MetricTag("").ListAll()

	// Verify we have all expected metrics (existing + new DB metrics)
	expectedCount := 8 + 7 // 8 existing + 7 new DB metrics
	assert.Equal(t, expectedCount, len(allTags),
		"ListAll() should return %d metrics", expectedCount)
}

func Test_MetricTag_Categorization(t *testing.T) {
	// Test that DB metrics are properly categorized
	gaugeMetrics := []MetricTag{
		DBOpenConnectionsTag,
		DBMaxOpenConnectionsTag,
		DBInUseConnectionsTag,
		DBIdleConnectionsTag,
	}

	counterMetrics := []MetricTag{
		DBWaitCountTotalTag,
		DBWaitDurationSecondsTotalTag,
		DBMaxIdleClosedTotalTag,
		DBMaxIdleTimeClosedTotalTag,
		DBMaxLifetimeClosedTotalTag,
	}

	// Verify gauge metrics have appropriate naming
	for _, gauge := range gaugeMetrics {
		assert.NotContains(t, string(gauge), "_total",
			"Gauge metric %s should not have '_total' suffix", gauge)
	}

	// Verify counter metrics have total suffix
	for _, counter := range counterMetrics {
		assert.Contains(t, string(counter), "_total",
			"Counter metric %s should have '_total' suffix", counter)
	}
}
