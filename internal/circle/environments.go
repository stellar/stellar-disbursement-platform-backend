package circle

// Environment holds the possible environments for the Circle API
type Environment string

const (
	Production Environment = "https://api.circle.com"
	Sandbox    Environment = "https://api-sandbox.circle.com"
)
