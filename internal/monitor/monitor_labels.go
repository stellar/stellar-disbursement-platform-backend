package monitor

type HTTPRequestLabels struct {
	Status string
	Route  string
	Method string
}

type DBQueryLabels struct {
	QueryType string
}

type DisbursementLabels struct {
	Asset  string
	Wallet string
}

func (d DisbursementLabels) ToMap() map[string]string {
	return map[string]string{
		"asset":  d.Asset,
		"wallet": d.Wallet,
	}
}

type CircleLabels struct {
	Method     string
	Endpoint   string
	Status     string
	StatusCode string
	TenantName string
}

func (c CircleLabels) ToMap() map[string]string {
	return map[string]string{
		"method":      c.Method,
		"endpoint":    c.Endpoint,
		"status":      c.Status,
		"status_code": c.StatusCode,
		"tenant_name": c.TenantName,
	}
}

var CircleLabelNames = []string{"method", "endpoint", "status", "status_code", "tenant_name"}
