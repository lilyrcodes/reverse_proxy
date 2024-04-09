package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"
	"time"
)

type UrlMapping struct {
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
	Port   uint16 `json:"port"`
}

func (m *UrlMapping) matches(u *url.URL) bool {
	return m.Host == u.Host && m.Scheme == u.Scheme
}

type TlsFiles struct {
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

type Config struct {
	Mapping   []UrlMapping `json:"mapping"`
	TlsFiles  []TlsFiles   `json:"tls_files"`
	HttpPort  uint16       `json:"http_port"`
	HttpsPort uint16       `json:"https_port"`
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

func shutdownFunc(other *http.Server, isOtherDone *atomic.Bool, complete chan bool) func() {
	return func() {
		if !isOtherDone.Swap(true) {
			if err := other.Shutdown(context.Background()); err != nil {
				panic(err)
			}
		} else {
			close(complete)
		}
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
				if m.matches(r.In.URL) {
					host := fmt.Sprintf("localhost:%d", m.Port)
					u := *r.In.URL
					u.Host = host
					r.SetURL(&u)
					r.SetXForwarded()
					return
				}
			}
			return
		},
	}
	httpServer := makeServer(config.HttpPort, proxy, config, nil)
	httpsServer := makeServer(config.HttpsPort, proxy, config, tlsConfig)

	var isOtherDone *atomic.Bool
	shutdownComplete := make(chan bool)
	httpServer.RegisterOnShutdown(shutdownFunc(httpsServer, isOtherDone, shutdownComplete))
	httpsServer.RegisterOnShutdown(shutdownFunc(httpServer, isOtherDone, shutdownComplete))

	go httpServer.ListenAndServe()
	go httpsServer.ListenAndServeTLS("", "")
	<-shutdownComplete
}

type ProxyHandler struct {
	p *httputil.ReverseProxy
	c *Config
}

func (ph *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	match := false
	for _, m := range ph.c.Mapping {
		if m.matches(r.URL) {
			match = true
			break
		}
	}
	if match {
		ph.p.ServeHTTP(w, r)
	} else {
		w.WriteHeader(http.StatusBadGateway)
	}
}
