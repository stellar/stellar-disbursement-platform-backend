package monitor

type MetricTag string

const (
	SuccessfulQueryDurationTag MetricTag = "successful_queries_duration"
	FailureQueryDurationTag    MetricTag = "failure_queries_duration"
	HttpRequestDurationTag     MetricTag = "requests_duration_seconds"
	// Disbursements:
	DisbursementsCounterTag MetricTag = "disbursements_counter"
	// AnchorPlatformAuthProtection
	AnchorPlatformAuthProtectionEnsuredCounterTag MetricTag = "anchor_platform_auth_protection_ensured_counter"
	AnchorPlatformAuthProtectionMissingCounterTag MetricTag = "anchor_platform_auth_protection_missing_counter"
)

func (m MetricTag) ListAll() []MetricTag {
	return []MetricTag{
		SuccessfulQueryDurationTag,
		FailureQueryDurationTag,
		HttpRequestDurationTag,
		DisbursementsCounterTag,
		AnchorPlatformAuthProtectionEnsuredCounterTag,
		AnchorPlatformAuthProtectionMissingCounterTag,
	}
}
