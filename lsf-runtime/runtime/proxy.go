package lsfrt

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The credential proxy (docs/design/lsf-runtime.md, Credential proxy): it
// listens on the address the config gives, for the launch's life, refuses a
// request whose bearer is not the launch's placeholder, and forwards the
// rest upstream with the live OAuth access token in its place. The token is
// re-read when the credentials file changes, and never logged.

// ProxyConfig is the config's proxy block.
type ProxyConfig struct {
	Listen          string `json:"listen"`             // 127.0.0.1:port; port 0 picks one
	CredentialsFile string `json:"credentialsFile"`    // the live session's .credentials.json
	Upstream        string `json:"upstream,omitempty"` // default https://api.anthropic.com
}

const (
	defaultUpstream = "https://api.anthropic.com"
	oauthBeta       = "oauth-2025-04-20"
	proxyRefusal    = "lsf-proxy: placeholder bearer required\n"
)

// ProxyInfo is what a call is given to reach the proxy: its base URL and the
// bearer it must present.
type ProxyInfo struct {
	URL         string
	Placeholder string
}

func (c *ProxyConfig) prepare() []error {
	var errs []error
	if c.Listen == "" {
		errs = append(errs, errors.New("proxy.listen is required"))
	} else if host, _, err := net.SplitHostPort(c.Listen); err != nil {
		errs = append(errs, fmt.Errorf("proxy.listen: %w", err))
	} else if host != "127.0.0.1" {
		// pasta splices a container's 127.0.0.1 to the host's.
		errs = append(errs, fmt.Errorf("proxy.listen: the host must be 127.0.0.1, not %q", host))
	}
	if c.CredentialsFile == "" {
		errs = append(errs, errors.New("proxy.credentialsFile is required"))
	}
	if c.Upstream == "" {
		c.Upstream = defaultUpstream
	}
	u, err := url.Parse(c.Upstream)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		errs = append(errs, fmt.Errorf("proxy.upstream must be an http(s) origin such as %s, not %q", defaultUpstream, c.Upstream))
	} else if ip := net.ParseIP(u.Hostname()); u.Scheme == "http" && (ip == nil || !ip.IsLoopback()) {
		// The token would cross the network in cleartext.
		errs = append(errs, fmt.Errorf("proxy.upstream: http is for a loopback address only, not %q", c.Upstream))
	}
	return errs
}

type proxy struct {
	rt          *Runtime
	srv         *http.Server
	ln          net.Listener
	done        chan struct{}
	placeholder string
	upstream    *url.URL
	credFile    string
	rp          *httputil.ReverseProxy
	tr          *http.Transport

	mu        sync.Mutex
	token     string
	stamp     fileStamp
	refusals  int
	reReadBad bool // the last re-read failed; logged once until one succeeds
}

// fileStamp is what a change of the credentials file is detected by.
type fileStamp struct {
	mtime time.Time
	size  int64
	ino   uint64
}

func stampOf(fi os.FileInfo) fileStamp {
	s := fileStamp{mtime: fi.ModTime(), size: fi.Size()}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		s.ino = uint64(st.Ino)
	}
	return s
}

// startProxy starts the proxy on the config's address. A bind failure or a
// credentials file without a token is a setup error.
func (rt *Runtime) startProxy() (*proxy, error) {
	c := rt.cfg.Proxy
	p := &proxy{rt: rt, credFile: c.CredentialsFile, done: make(chan struct{})}
	p.upstream, _ = url.Parse(c.Upstream) // checked in prepare
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	p.placeholder = hex.EncodeToString(b)
	if err := p.reload(true); err != nil {
		return nil, err
	}
	errLog := log.New(logWriter{rt}, "proxy: ", 0)
	// The default transport's settings (an https upstream reached through
	// HTTPS_PROXY by a CONNECT tunnel), with a bound on the upstream's
	// response headers; a response may stream for long.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 10 * time.Minute
	p.rp = &httputil.ReverseProxy{
		Rewrite:       p.rewrite,
		Transport:     tr,
		FlushInterval: -1,
		ErrorLog:      errLog,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() == nil { // not the client going away
				rt.Log(fmt.Sprintf("proxy: %q %q: upstream: %v", r.Method, r.URL.Path, err))
			}
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	p.tr = tr
	ln, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return nil, err
	}
	p.ln = ln
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute, ErrorLog: errLog}
	go func() {
		defer close(p.done)
		if err := p.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			rt.Log("proxy: " + err.Error())
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	rt.Log(fmt.Sprintf("proxy: listening on %s for %s", addr, p.upstream))
	rt.event("proxy", map[string]any{"listen": addr.String(), "upstream": p.upstream.String()})
	return p, nil
}

// close stops the proxy at launch end: it stops accepting, waits briefly
// for requests in flight, then closes what is left.
func (p *proxy) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if p.srv.Shutdown(ctx) != nil {
		p.srv.Close()
	}
	<-p.done
	p.tr.CloseIdleConnections()
	p.mu.Lock()
	n := p.refusals
	p.mu.Unlock()
	if n > 0 {
		p.rt.Log(fmt.Sprintf("proxy: %d request(s) refused this launch", n))
	}
}

func (p *proxy) info() ProxyInfo {
	return ProxyInfo{URL: "http://" + p.ln.Addr().String(), Placeholder: p.placeholder}
}

// Proxy is the running proxy's URL and placeholder. Run starts the proxy
// before the body, so a body never sees the zero value.
func (rt *Runtime) Proxy() ProxyInfo {
	if rt.proxy == nil {
		return ProxyInfo{}
	}
	return rt.proxy.info()
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	why := p.refuse(r.Header.Values("Authorization"))
	if why == "" && r.Method == http.MethodConnect {
		why = "CONNECT"
	}
	if why != "" {
		p.mu.Lock()
		p.refusals++
		first := p.refusals == 1
		p.mu.Unlock()
		if first {
			p.rt.Log(fmt.Sprintf("proxy: refused %q %q: %s (logged once per launch)", r.Method, r.URL.Path, why))
			p.rt.event("proxy-refused", map[string]any{"method": r.Method, "path": r.URL.Path, "why": why})
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, proxyRefusal)
		return
	}
	p.rp.ServeHTTP(w, r)
}

// refuse says why a request's Authorization headers are not exactly the
// placeholder bearer, "" when they are. Never the header's value.
func (p *proxy) refuse(auth []string) string {
	switch {
	case len(auth) == 0:
		return "no bearer"
	case len(auth) > 1:
		return "more than one Authorization header"
	case strings.TrimSpace(auth[0]) == "" || strings.TrimSpace(auth[0]) == "Bearer":
		return "empty bearer"
	case subtle.ConstantTimeCompare([]byte(auth[0]), []byte("Bearer "+p.placeholder)) != 1:
		return "not the launch's placeholder"
	}
	return ""
}

func (p *proxy) rewrite(pr *httputil.ProxyRequest) {
	pr.SetURL(p.upstream)
	pr.Out.Header.Del("X-Api-Key")
	pr.Out.Header.Del("Proxy-Authorization")
	pr.Out.Header.Set("Authorization", "Bearer "+p.currentToken())
	betas := pr.Out.Header.Values("Anthropic-Beta")
	for _, v := range betas {
		for _, b := range strings.Split(v, ",") {
			if strings.TrimSpace(b) == oauthBeta {
				return
			}
		}
	}
	pr.Out.Header.Set("Anthropic-Beta", strings.Join(append(betas, oauthBeta), ","))
}

// currentToken re-reads the credentials file when it has changed since the
// last read and returns the token in force: a failed re-read keeps the one
// before.
func (p *proxy) currentToken() string {
	err := p.reload(false)
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case err != nil && !p.reReadBad:
		p.reReadBad = true
		p.rt.Log("proxy: credentials re-read failed; the previous token stays until a re-read succeeds: " + err.Error())
	case err == nil && p.reReadBad:
		p.reReadBad = false
		p.rt.Log("proxy: credentials re-read succeeded")
	}
	return p.token
}

// reload reads the token when the file's stamp differs from the last read's
// (always when first). An error names the file, never the token.
func (p *proxy) reload(first bool) error {
	fi, err := os.Stat(p.credFile)
	if err != nil {
		return fmt.Errorf("credentials file: %w", err)
	}
	st := stampOf(fi)
	p.mu.Lock()
	unchanged := !first && st == p.stamp
	p.mu.Unlock()
	if unchanged {
		return nil
	}
	b, err := os.ReadFile(p.credFile)
	if err != nil {
		return fmt.Errorf("credentials file: %w", err)
	}
	var d struct {
		ClaudeAiOauth *struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return fmt.Errorf("credentials file %s is not JSON", p.credFile)
	}
	tok := d.AccessToken
	if d.ClaudeAiOauth != nil && d.ClaudeAiOauth.AccessToken != "" {
		tok = d.ClaudeAiOauth.AccessToken
	}
	if tok == "" || strings.ContainsAny(tok, "\r\n") {
		return fmt.Errorf("credentials file %s holds no access token", p.credFile)
	}
	p.mu.Lock()
	changed := p.token != "" && p.token != tok
	p.token, p.stamp = tok, st
	p.mu.Unlock()
	if changed {
		p.rt.Log("proxy: credentials file changed; the new token is in use")
		p.rt.event("proxy-token-reloaded", nil)
	}
	return nil
}

// logWriter sends a log.Logger's lines to rt.Log.
type logWriter struct{ rt *Runtime }

func (w logWriter) Write(b []byte) (int, error) {
	w.rt.Log(strings.TrimRight(string(b), "\n"))
	return len(b), nil
}
