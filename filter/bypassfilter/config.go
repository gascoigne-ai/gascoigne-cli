package bypassfilter

const DefaultEnv = "GASCOIGNE_RAW"

type Config struct {
	// Env names a variable that, when truthy, bypasses optimization for every payload.
	Env string `yaml:"env"`
}
