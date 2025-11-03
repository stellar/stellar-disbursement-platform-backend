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

	// Connection pool gauges (real-time state)
	DBOpenConnectionsTag    MetricTag = "open_connections"
	DBInUseConnectionsTag   MetricTag = "in_use_connections"
	DBIdleConnectionsTag    MetricTag = "idle_connections"
	DBMaxOpenConnectionsTag MetricTag = "max_open_connections"

	// Connection pool counters (cumulative)
	DBWaitCountTotalTag           MetricTag = "wait_count_total"
	DBWaitDurationSecondsTotalTag MetricTag = "wait_duration_seconds_total"
	DBMaxIdleClosedTotalTag       MetricTag = "max_idle_closed_total"
	DBMaxIdleTimeClosedTotalTag   MetricTag = "max_idle_time_closed_total"
	DBMaxLifetimeClosedTotalTag   MetricTag = "max_lifetime_closed_total"
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
}
