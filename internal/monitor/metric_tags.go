package monitor

type MetricTag string

const (
	SuccessfulQueryDurationTag MetricTag = "successful_queries_duration"
	FailureQueryDurationTag    MetricTag = "failure_queries_duration"
	HTTPRequestDurationTag     MetricTag = "requests_duration_seconds"
	// Disbursements:
	DisbursementsCounterTag MetricTag = "disbursements_counter"
	// AnchorPlatformAuthProtection
	AnchorPlatformAuthProtectionEnsuredCounterTag MetricTag = "anchor_platform_auth_protection_ensured_counter"
	AnchorPlatformAuthProtectionMissingCounterTag MetricTag = "anchor_platform_auth_protection_missing_counter"
	// Circle API Requests
	CircleAPIRequestDurationTag MetricTag = "circle_api_request_duration_seconds"
	CircleAPIRequestsTotalTag   MetricTag = "circle_api_requests_total"
)

func (m MetricTag) ListAll() []MetricTag {
	return []MetricTag{
		SuccessfulQueryDurationTag,
		FailureQueryDurationTag,
		HTTPRequestDurationTag,
		DisbursementsCounterTag,
		AnchorPlatformAuthProtectionEnsuredCounterTag,
		AnchorPlatformAuthProtectionMissingCounterTag,
		CircleAPIRequestDurationTag,
		CircleAPIRequestsTotalTag,
	}
}
