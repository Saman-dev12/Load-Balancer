package config

type Config struct {
	Backends         []Backend
	HealthCheckRoute string
	Duration         int
	Algorithm        string
}

func (c *Config) CheckAndCorrectConfig() {
	if c.HealthCheckRoute == "" {
		c.HealthCheckRoute = "/health"
	}

	if c.Duration == 0 {
		c.Duration = 300
	}

	if c.Algorithm == "" {
		c.Algorithm = "Round Robin"
	}
}
