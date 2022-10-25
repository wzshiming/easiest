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

func dialTarget(o string) (net.Conn, string, error) {
	u, err := url.Parse(o)
	if err != nil {
		return nil, "", err
	}
	switch u.Scheme {
	case "http":
		port := u.Port()
		if port == "" {
			port = "80"
		}
		host := u.Hostname()
		conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
		if err != nil {
			return nil, "", err
		}
		return conn, host, nil
	case "https":
		port := u.Port()
		if port == "" {
			port = "443"
		}
		host := u.Hostname()
		conn, err := tls.Dial("tcp", net.JoinHostPort(host, port), &tls.Config{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return nil, "", err
		}
		return conn, host, nil
	}
	return nil, "", fmt.Errorf("unsupported scheme %q", u.Scheme)
}
