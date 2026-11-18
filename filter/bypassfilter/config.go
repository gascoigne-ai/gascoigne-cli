package bypassfilter

const defaultEnv = "GASCOIGNE_RAW"

// Config names the environment variable that bypasses optimization.
type Config struct {
	// Env names a variable that, when truthy, bypasses optimization for every payload.
	Env string `yaml:"env"`
}
