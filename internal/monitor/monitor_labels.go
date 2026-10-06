package monitor

type CommonLabels struct {
	TenantName string
}

type HTTPRequestLabels struct {
	Status string
	Route  string
	Method string
	CommonLabels
}

type DBQueryLabels struct {
	QueryType string
}

type DisbursementLabels struct {
	Asset  string
	Wallet string
	// WalletID is the source DISTRIBUTION wallet UUID (per-wallet observability) —
	// distinct from Wallet, which is the recipient wallet-provider name.
	WalletID string
	CommonLabels
}

func (d DisbursementLabels) ToMap() map[string]string {
	return map[string]string{
		"asset":       d.Asset,
		"wallet":      d.Wallet,
		"wallet_id":   d.WalletID,
		"tenant_name": d.TenantName,
	}
}

type CircleLabels struct {
	Method     string
	Endpoint   string
	Status     string
	StatusCode string
	CommonLabels
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
