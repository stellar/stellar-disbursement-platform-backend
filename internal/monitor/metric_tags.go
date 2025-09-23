package monitor

type MetricTag string

const (
	SuccessfulQueryDurationTag MetricTag = "successful_queries_duration"
	FailureQueryDurationTag    MetricTag = "failure_queries_duration"
	HttpRequestDurationTag     MetricTag = "requests_duration_seconds"
	// Disbursements:
	DisbursementsCounterTag MetricTag = "disbursements_counter"
	// Circle API Requests
	CircleAPIRequestDurationTag MetricTag = "circle_api_request_duration_seconds"
	CircleAPIRequestsTotalTag   MetricTag = "circle_api_requests_total"
)

func (m MetricTag) ListAll() []MetricTag {
	return []MetricTag{
		SuccessfulQueryDurationTag,
		FailureQueryDurationTag,
		HttpRequestDurationTag,
		DisbursementsCounterTag,
		CircleAPIRequestDurationTag,
		CircleAPIRequestsTotalTag,
	}
}
