package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

type UrlMapping struct {
	Host   string `json:"host"`
	Scheme string `json:"schema"`
	Port   uint16 `json:"port"`
}

func (m *UrlMapping) matches(u *url.URL) bool {
	return m.Host == u.Host && m.Scheme == u.Scheme
}

type Config struct {
	Mapping    []UrlMapping `json:"mapping"`
	ListenPort uint16       `json:"listen_port"`
}

func ReadConfig(fname string) (Config, error) {
	var config Config
	b, err := os.ReadFile(fname)
	if err != nil {
		return Config{}, err
	}
	err = json.Unmarshal(b, &config)
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: reverse_proxy <config_file>")
		return
	}
	config_file := os.Args[1]
	config, err := ReadConfig(config_file)
	if err != nil {
		panic(err)
	}

	proxy := httputil.ReverseProxy{
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

	http.Handle("/", &ProxyHandler{&proxy})
	err = http.ListenAndServe(fmt.Sprintf(":%d", config.ListenPort), nil)
	if err != nil {
		panic(err)
	}
}

type ProxyHandler struct {
	p *httputil.ReverseProxy
}

func (ph *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ph.p.ServeHTTP(w, r)
}
