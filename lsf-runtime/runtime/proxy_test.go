package lsfrt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeCreds writes a credentials file holding tok as the OAuth access
// token, as Claude Code's .credentials.json does.
func writeCreds(t *testing.T, path, tok string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{"accessToken": tok, "refreshToken": "r"}})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// upstreamSeen is what the test upstream received.
type upstreamSeen struct {
	mu   sync.Mutex
	reqs []*http.Request
	body []string
}

func (u *upstreamSeen) n() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.reqs)
}

// proxyConfig is testConfig with a proxy in front of an upstream that
// records each request and answers 200 "ok".
func proxyConfig(t *testing.T, tok string) (Config, string, *upstreamSeen) {
	t.Helper()
	cfg, _ := testConfig(t)
	seen := &upstreamSeen{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen.mu.Lock()
		seen.reqs = append(seen.reqs, r)
		seen.body = append(seen.body, string(b))
		seen.mu.Unlock()
		io.WriteString(w, "ok")
	}))
	t.Cleanup(up.Close)
	creds := filepath.Join(t.TempDir(), ".credentials.json")
	writeCreds(t, creds, tok)
	cfg.Proxy = &ProxyConfig{Listen: "127.0.0.1:0", CredentialsFile: creds, Upstream: up.URL}
	return cfg, creds, seen
}

func proxyReq(t *testing.T, p ProxyInfo, auth []string, hdr map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", p.URL+"/v1/messages?beta=true", strings.NewReader(`{"x":1}`))
	for _, a := range auth {
		req.Header.Add("Authorization", a)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// stateFiles is log.txt and telemetry.jsonl together.
func stateFiles(t *testing.T, cfg Config) string {
	t.Helper()
	var all string
	for _, f := range []string{"log.txt", "telemetry.jsonl"} {
		b, err := os.ReadFile(filepath.Join(cfg.StateDir, f))
		if err != nil {
			t.Fatal(err)
		}
		all += string(b)
	}
	return all
}

// Any request without exactly the placeholder bearer is refused, and none
// reaches the upstream; the first refusal and the count are logged, the
// header's value never.
func TestProxyRefuses(t *testing.T) {
	cfg, _, seen := proxyConfig(t, "live-token-1")
	var p ProxyInfo
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		p = rt.Proxy()
		if len(p.Placeholder) != 64 || !strings.HasPrefix(p.URL, "http://127.0.0.1:") {
			t.Fatalf("proxy info %+v", p)
		}
		for name, auth := range map[string][]string{
			"absent":        nil,
			"empty":         {""},
			"empty bearer":  {"Bearer "},
			"wrong":         {"Bearer not-the-placeholder"},
			"placeholder+1": {"Bearer " + p.Placeholder + "x"},
			"no scheme":     {p.Placeholder},
			"two headers":   {"Bearer " + p.Placeholder, "Bearer " + p.Placeholder},
		} {
			code, body := proxyReq(t, p, auth, nil)
			if code != http.StatusForbidden || body != proxyRefusal {
				t.Errorf("%s: %d %q", name, code, body)
			}
		}
		return nil, nil
	})
	if seen.n() != 0 {
		t.Errorf("%d refused request(s) reached the upstream", seen.n())
	}
	logs := stateFiles(t, cfg)
	if strings.Count(logs, "proxy: refused") != 1 || !strings.Contains(logs, "proxy: 7 request(s) refused this launch") {
		t.Errorf("refusal logging:\n%s", logs)
	}
	if strings.Contains(logs, "not-the-placeholder") || strings.Contains(logs, p.Placeholder) || strings.Contains(logs, "live-token-1") {
		t.Error("a bearer or the token reached the logs")
	}
}

// The placeholder is replaced by the live token, x-api-key is dropped, and
// the OAuth beta is added once; method, path, query and body pass through.
func TestProxySubstitutes(t *testing.T) {
	cfg, _, seen := proxyConfig(t, "live-token-1")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		p := rt.Proxy()
		bearer := []string{"Bearer " + p.Placeholder}
		if code, body := proxyReq(t, p, bearer, map[string]string{"X-Api-Key": "k", "Anthropic-Beta": "a-1"}); code != 200 || body != "ok" {
			t.Fatalf("%d %q", code, body)
		}
		proxyReq(t, p, bearer, map[string]string{"Anthropic-Beta": "oauth-2025-04-20,b-2"})
		proxyReq(t, p, bearer, nil)
		return nil, nil
	})
	if seen.n() != 3 {
		t.Fatalf("%d requests upstream", seen.n())
	}
	r := seen.reqs[0]
	if r.Method != "POST" || r.URL.Path != "/v1/messages" || r.URL.RawQuery != "beta=true" || seen.body[0] != `{"x":1}` {
		t.Errorf("request: %s %s %q", r.Method, r.URL, seen.body[0])
	}
	if got := r.Header.Values("Authorization"); len(got) != 1 || got[0] != "Bearer live-token-1" {
		t.Errorf("authorization: %q", got)
	}
	if r.Header.Get("X-Api-Key") != "" {
		t.Error("x-api-key forwarded")
	}
	for i, want := range []string{"a-1,oauth-2025-04-20", "oauth-2025-04-20,b-2", "oauth-2025-04-20"} {
		if got := seen.reqs[i].Header.Values("Anthropic-Beta"); len(got) != 1 || got[0] != want {
			t.Errorf("request %d beta: %q, want %q", i, got, want)
		}
	}
	if strings.Contains(stateFiles(t, cfg), "live-token-1") {
		t.Error("the token reached the logs")
	}
}

// The token is re-read when the file is replaced or rewritten in place; a
// file that no longer holds one keeps the previous token.
func TestProxyRereads(t *testing.T) {
	cfg, creds, seen := proxyConfig(t, "token-one")
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		p := rt.Proxy()
		bearer := []string{"Bearer " + p.Placeholder}
		proxyReq(t, p, bearer, nil)
		// Replaced by rename, as an atomic writer does.
		tmp := creds + ".new"
		writeCreds(t, tmp, "token-two-longer")
		if err := os.Rename(tmp, creds); err != nil {
			t.Fatal(err)
		}
		proxyReq(t, p, bearer, nil)
		// Rewritten in place.
		writeCreds(t, creds, "token-3")
		proxyReq(t, p, bearer, nil)
		// Broken: the previous token stays.
		os.WriteFile(creds, []byte("{not json"), 0o600)
		proxyReq(t, p, bearer, nil)
		return nil, nil
	})
	for i, want := range []string{"token-one", "token-two-longer", "token-3", "token-3"} {
		if got := seen.reqs[i].Header.Get("Authorization"); got != "Bearer "+want {
			t.Errorf("request %d: %q, want %q", i, got, want)
		}
	}
	logs := stateFiles(t, cfg)
	if !strings.Contains(logs, "credentials re-read failed; the previous token stays") || !strings.Contains(logs, "the new token is in use") {
		t.Errorf("re-read logging:\n%s", logs)
	}
	for _, tok := range []string{"token-one", "token-two-longer", "token-3"} {
		if strings.Contains(logs, tok) {
			t.Errorf("%s reached the logs", tok)
		}
	}
}

// The response streams unbuffered: the client reads the upstream's first
// chunk while the upstream is still blocked.
func TestProxyStreams(t *testing.T) {
	cfg, _ := testConfig(t)
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: one\n\n")
		w.(http.Flusher).Flush()
		<-release
		io.WriteString(w, "event: two\n\n")
	}))
	defer up.Close()
	defer close(release)
	creds := filepath.Join(t.TempDir(), "c.json")
	writeCreds(t, creds, "tok")
	cfg.Proxy = &ProxyConfig{Listen: "127.0.0.1:0", CredentialsFile: creds, Upstream: up.URL}
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		p := rt.Proxy()
		req, _ := http.NewRequest("POST", p.URL+"/v1/messages", nil)
		req.Header.Set("Authorization", "Bearer "+p.Placeholder)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		got := make(chan string, 1)
		go func() {
			buf := make([]byte, 64)
			n, _ := resp.Body.Read(buf)
			got <- string(buf[:n])
		}()
		select {
		case s := <-got:
			if s != "event: one\n\n" {
				t.Errorf("first chunk %q", s)
			}
		case <-time.After(5 * time.Second):
			t.Error("the first chunk did not arrive before the upstream finished")
		}
		release <- struct{}{}
		rest, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(rest) != "event: two\n\n" {
			t.Errorf("rest %q", rest)
		}
		return nil, nil
	})
}

// The proxy listens on the address given while the launch runs, and the
// port refuses once Run returns. An upstream that cannot be reached is a
// 502.
func TestProxyLifecycle(t *testing.T) {
	cfg, _, _ := proxyConfig(t, "tok")
	dead, _ := net.Listen("tcp", "127.0.0.1:0")
	deadURL := "http://" + dead.Addr().String()
	dead.Close()
	cfg.Proxy.Upstream = deadURL
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	cfg.Proxy.Listen = addr
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		p := rt.Proxy()
		if p.URL != "http://"+addr {
			t.Errorf("url %s, want the given address %s", p.URL, addr)
		}
		if code, _ := proxyReq(t, p, []string{"Bearer " + p.Placeholder}, nil); code != http.StatusBadGateway {
			t.Errorf("dead upstream: %d", code)
		}
		return nil, nil
	})
	if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		c.Close()
		t.Error("the proxy port still accepts after the launch")
	}
	if !strings.Contains(stateFiles(t, cfg), "upstream:") {
		t.Error("the upstream error is not logged")
	}
}

// No token, an unreadable file, or a port in use is a setup error.
func TestProxySetupErrors(t *testing.T) {
	busy, _ := net.Listen("tcp", "127.0.0.1:0")
	defer busy.Close()
	for name, setup := range map[string]func(cfg *Config, creds string){
		"no token":     func(_ *Config, creds string) { os.WriteFile(creds, []byte(`{"claudeAiOauth":{}}`), 0o600) },
		"missing file": func(_ *Config, creds string) { os.Remove(creds) },
		"port in use":  func(cfg *Config, _ string) { cfg.Proxy.Listen = busy.Addr().String() },
	} {
		t.Run(name, func(t *testing.T) {
			cfg, creds, _ := proxyConfig(t, "tok")
			setup(&cfg, creds)
			ran := false
			_, err := Run(context.Background(), cfg, func(context.Context, *Runtime) (json.RawMessage, error) { ran = true; return nil, nil })
			if err == nil || ran || !strings.Contains(err.Error(), "proxy") {
				t.Errorf("err %v, body ran %v", err, ran)
			}
		})
	}
	// The top-level accessToken form is read too.
	cfg, creds, seen := proxyConfig(t, "x")
	os.WriteFile(creds, []byte(`{"accessToken":"top-level"}`), 0o600)
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		proxyReq(t, rt.Proxy(), []string{"Bearer " + rt.Proxy().Placeholder}, nil)
		return nil, nil
	})
	if seen.reqs[0].Header.Get("Authorization") != "Bearer top-level" {
		t.Error("top-level accessToken not read")
	}
}

func TestProxyConfig(t *testing.T) {
	for _, c := range []struct {
		p    ProxyConfig
		want string
	}{
		{ProxyConfig{CredentialsFile: "c"}, "proxy.listen is required"},
		{ProxyConfig{Listen: "nope", CredentialsFile: "c"}, "proxy.listen"},
		{ProxyConfig{Listen: "127.0.0.1:0"}, "proxy.credentialsFile is required"},
		{ProxyConfig{Listen: "127.0.0.1:0", CredentialsFile: "c", Upstream: "ftp://x"}, "proxy.upstream must be"},
		{ProxyConfig{Listen: "127.0.0.1:0", CredentialsFile: "c", Upstream: "https://x/v1"}, "proxy.upstream must be"},
	} {
		if err := errors.Join(c.p.prepare()...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v", c.p, err)
		}
	}
	p := ProxyConfig{Listen: "127.0.0.1:0", CredentialsFile: "c"}
	if errs := p.prepare(); len(errs) != 0 || p.Upstream != defaultUpstream {
		t.Errorf("default upstream: %v %s", errs, p.Upstream)
	}
	var none *Runtime = &Runtime{}
	if none.Proxy() != (ProxyInfo{}) {
		t.Error("no proxy, empty info")
	}
}
