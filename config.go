package easiest

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
)

type Config struct {
	TlsDir string  `yaml:"tlsDir,omitempty"`
	Routes []Route `yaml:"routes,omitempty"`
}

type HttpConfig struct {
	ForceTLS           bool   `yaml:"forceTLS,omitempty"`
	HeaderForwardedFor bool   `yaml:"headerForwardedFor,omitempty"`
	AcceptEncoding     string `yaml:"acceptEncoding,omitempty"`
}

type Route struct {
	Domain   string     `yaml:"domain,omitempty"`
	Target   string     `yaml:"target,omitempty"`
	HTTP     HttpConfig `yaml:"http,omitempty"`
	Replaces []Replace  `yaml:"replaces,omitempty"`
}

type Replace struct {
	Old string `yaml:"old,omitempty"`
	New string `yaml:"new,omitempty"`
}

func dialTarget(o string) (net.Conn, error) {
	u, err := url.Parse(o)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http":
		port := u.Port()
		if port == "" {
			port = "80"
		}
		return net.Dial("tcp", net.JoinHostPort(u.Hostname(), port))
	case "https":
		port := u.Port()
		if port == "" {
			port = "443"
		}
		return tls.Dial("tcp", net.JoinHostPort(u.Hostname(), port), &tls.Config{
			InsecureSkipVerify: true,
		})
	}
	return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
}
