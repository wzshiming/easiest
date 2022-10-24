package easiest

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/crypto/acme/autocert"
)

func newAcme(domains []string, dir string) *tls.Config {
	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
	}
	if len(domains) > 0 {
		m.HostPolicy = autocert.HostWhitelist(domains...)
	}
	if dir == "" {
		dir = cacheDir()
	}
	m.Cache = autocert.DirCache(dir)
	tlsConfig := m.TLSConfig()
	tlsConfig.NextProtos = removeOneFromSet(tlsConfig.NextProtos, "h2")
	return tlsConfig
}

func removeOneFromSet(set []string, o string) []string {
	for i, s := range set {
		if s == o {
			return append(set[:i], set[i+1:]...)
		}
	}
	return set
}

func homeDir() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
	}
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return "/"
}

func cacheDir() string {
	const base = "autocert"
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Caches", base)
	case "windows":
		for _, ev := range []string{"APPDATA", "CSIDL_APPDATA", "TEMP", "TMP"} {
			if v := os.Getenv(ev); v != "" {
				return filepath.Join(v, base)
			}
		}
		// Worst case:
		return filepath.Join(homeDir(), base)
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, base)
	}
	return filepath.Join(homeDir(), ".cache", base)
}
