package monitor

type HttpRequestLabels struct {
	Status string
	Route  string
	Method string
}

type DBQueryLabels struct {
	QueryType string
}

type DisbursementLabels struct {
	Asset   string
	Country string
	Wallet  string
}

func (d DisbursementLabels) ToMap() map[string]string {
	return map[string]string{
		"asset":   d.Asset,
		"country": d.Country,
		"wallet":  d.Wallet,
	}
}
