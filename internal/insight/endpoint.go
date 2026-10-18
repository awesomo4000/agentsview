package insight

// EndpointConfig is retained to recognize obsolete configuration. No transport remains.
type EndpointConfig struct {
	Endpoint  string
	Model     string
	APIKey    string
	AllowHTTP bool
}
