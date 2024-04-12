package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"
)

type UrlMapping struct {
	Host   string `json:"host"`
	UseTls bool   `json:"use_tls"`
	Port   uint16 `json:"port"`
}

func (m *UrlMapping) matches(r *http.Request) bool {
	hostName := strings.Split(r.Host, ":")[0]
	return m.Host == hostName && (r.TLS != nil) == m.UseTls
}

type TlsFiles struct {
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

type Config struct {
	Mapping  []UrlMapping `json:"mapping"`
	TlsFiles []TlsFiles   `json:"tls_files"`
	Port     uint16       `json:"port"`
}

func readConfig(fname string) (*Config, error) {
	config := &Config{}
	b, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (c *Config) LoadTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	if len(c.TlsFiles) > 0 {
		tlsConfig.Certificates = make([]tls.Certificate, len(c.TlsFiles))
	}
	var err error
	for i, pair := range c.TlsFiles {
		tlsConfig.Certificates[i], err = tls.LoadX509KeyPair(pair.CertFile, pair.KeyFile)
		if err != nil {
			return nil, err
		}
	}
	return tlsConfig, nil
}

func makeServer(port uint16, proxy *httputil.ReverseProxy, config *Config, tlsConfig *tls.Config) *http.Server {
	return &http.Server{
		Addr:           fmt.Sprintf("localhost:%d", port),
		Handler:        &ProxyHandler{proxy, config},
		TLSConfig:      tlsConfig,
		ReadTimeout:    time.Second * 15,
		WriteTimeout:   time.Second * 15,
		MaxHeaderBytes: 2 >> 16,
	}

}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: reverse_proxy <config_file>")
		return
	}
	configFile := os.Args[1]
	config, err := readConfig(configFile)
	if err != nil {
		panic(err)
	}
	tlsConfig, err := config.LoadTLSConfig()
	if err != nil {
		panic(err)
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			for _, m := range config.Mapping {
				if m.matches(r.In) {
					host := fmt.Sprintf("localhost:%d", m.Port)
					u := *r.In.URL
					u.Host = host
					r.Out.Host = host
					r.Out.TLS = r.In.TLS
					r.SetURL(&u)
					if r.In.TLS == nil {
						r.Out.URL.Scheme = "http"
					} else {
						r.Out.URL.Scheme = "https"
					}
					r.SetXForwarded()
					return
				}
			}
			return
		},
	}
	httpServer := makeServer(config.Port, proxy, config, tlsConfig)

	err = httpServer.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

type ProxyHandler struct {
	p *httputil.ReverseProxy
	c *Config
}

func (ph *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	match := false
	fmt.Print("Host: ", r.URL.Host, "Scheme: ", r.URL.Scheme, "Hostname: ", r.URL.Hostname())
	fmt.Println()
	if r.TLS != nil {
		fmt.Println("TLS connection")
	} else {
		fmt.Println("Non-TLS connection")
	}
	fmt.Print("Host: ", r.Host, "Hostname: ", r.URL.Hostname())
	fmt.Println()
	for _, m := range ph.c.Mapping {
		if m.matches(r) {
			match = true
			break
		}
	}
	if match {
		ph.p.ServeHTTP(w, r)
		fmt.Printf("Redirecting URL: %s\n", r.URL.String())
	} else {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Printf("Error serving URL: %s\n", r.URL.String())
	}
}
