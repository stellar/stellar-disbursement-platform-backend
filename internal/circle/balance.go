package circle

// Balance represents the amount and currency of a balance or transfer.
type Balance struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}
