package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	remote, err := url.Parse("http://google.com")
	if err != nil {
		panic(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(remote)
	http.Handle("/", &ProxyHandler{proxy, remote})
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}

type ProxyHandler struct {
	p      *httputil.ReverseProxy
	remote *url.URL
}

func (ph *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	r.Host = ph.remote.Host
	ph.p.ServeHTTP(w, r)
}
