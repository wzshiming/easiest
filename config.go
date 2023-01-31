package easiest

type Config struct {
	DebugAddress string  `yaml:"debugAddress,omitempty"`
	TlsDir       string  `yaml:"tlsDir,omitempty"`
	Routes       []Route `yaml:"routes,omitempty"`
}

type HttpConfig struct {
	ForceTLS           bool `yaml:"forceTLS,omitempty"`
	HeaderForwardedFor bool `yaml:"headerForwardedFor,omitempty"`
}

type Route struct {
	Domain   string     `yaml:"domain,omitempty"`
	Target   string     `yaml:"target,omitempty"`
	HTTP     HttpConfig `yaml:"http,omitempty"`
	Replaces []Replace  `yaml:"replaces,omitempty"`
	Stream   bool       `yaml:"stream,omitempty"`
}

type Replace struct {
	Old string `yaml:"old,omitempty"`
	New string `yaml:"new,omitempty"`
}
