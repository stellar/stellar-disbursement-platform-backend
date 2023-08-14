package monitor

type MetricTag string

const (
	SuccessfulQueryDurationTag MetricTag = "successful_queries_duration"
	FailureQueryDurationTag    MetricTag = "failure_queries_duration"
	HttpRequestDurationTag     MetricTag = "requests_duration_seconds"
	DisbursementsCounterTag    MetricTag = "disbursements_counter"
)

func (m MetricTag) ListAll() []MetricTag {
	return []MetricTag{
		SuccessfulQueryDurationTag,
		FailureQueryDurationTag,
		HttpRequestDurationTag,
		DisbursementsCounterTag,
	}
}
