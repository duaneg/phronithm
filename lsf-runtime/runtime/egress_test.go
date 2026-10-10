package lsfrt

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEgressAllowed(t *testing.T) {
	e := &egressProxy{allow: map[string]bool{}}
	for _, h := range []string{"Registry.NPMjs.org.", "[2001:DB8::1]", "192.0.2.1", "*.github.com"} {
		e.allow[normaliseHost(h)] = true
	}
	for _, tc := range []struct {
		target, host string
		ok           bool
	}{
		{"registry.npmjs.org:443", "registry.npmjs.org", true},
		{"REGISTRY.npmjs.ORG:443", "registry.npmjs.org", true},
		{"registry.npmjs.org.:443", "registry.npmjs.org", true},
		{"192.0.2.1:443", "192.0.2.1", true},
		{"[2001:db8::1]:443", "2001:db8::1", true},
		{"api.github.com:443", "api.github.com", true},
		{"a.b.github.com:443", "a.b.github.com", true},
		{"registry.npmjs.org:80", "", false},
		{"registry.npmjs.org:4430", "", false},
		{"registry.npmjs.org", "", false}, // no port
		{"example.com:443", "", false},
		{"npmjs.org:443", "", false},
		{"github.com:443", "", false},         // the apex of a *. entry
		{"evilgithub.com:443", "", false},     // a suffix lookalike
		{".github.com:443", "", false},        // no label before the suffix
		{"a..github.com:443", "", false},      // an empty label before it
		{"192.0.2.2:443", "", false},          // an unlisted IP
		{"[2001:db8::2]:443", "", false},      // an unlisted IPv6
		{"[::ffff:192.0.2.1]:443", "", false}, // another spelling of a listed IP
	} {
		host, ok := e.allowed(tc.target)
		if ok != tc.ok || host != tc.host {
			t.Errorf("%s: allowed %q %v, want %q %v", tc.target, host, ok, tc.host, tc.ok)
		}
	}
	// An IP target never matches a wildcard or a name entry.
	w := &egressProxy{allow: map[string]bool{"*.2.1": true, "*.0.2.1": true}}
	if _, ok := w.allowed("192.0.2.1:443"); ok {
		t.Error("an IP target matched a wildcard")
	}
}

// egressRequest sends a request line through the proxy, then extra, and
// returns the status line and the connection, positioned after the
// response headers.
func egressRequest(t *testing.T, addr, line string, extra string) (string, *bufio.Reader, net.Conn) {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(10 * time.Second))
	host, _, _ := strings.Cut(strings.TrimPrefix(strings.Fields(line)[1], "http://"), "/")
	if _, err := io.WriteString(c, line+"\r\nHost: "+host+"\r\n\r\n"+extra); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	for {
		l, err := br.ReadString('\n')
		if err != nil || l == "\r\n" {
			break
		}
	}
	return strings.TrimSpace(status), br, c
}

// echoServer echoes each connection, upper-casing nothing, and returns its
// address.
func echoServer(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	return ln.Addr().String()
}

// The proxy end to end: allowed CONNECTs tunnel to the normalised target
// name on 443, bytes sent with the CONNECT included; the rest are refused
// with 403 and a failed dial is 502. The first of each decision is an event.
func TestEgressProxy(t *testing.T) {
	cfg, _ := testConfig(t)
	echo := echoServer(t)
	var mu sync.Mutex
	var dialled []string
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.PinHosts([]string{"Registry.npmjs.org.", "*.github.com", "down.example"})
		e := rt.egress.Load()
		if e == nil {
			t.Fatal("no egress proxy with hosts pinned")
		}
		if !strings.HasPrefix(e.ln.Addr().String(), "127.0.0.1:") || rt.egressPort() == "0" {
			t.Errorf("listening on %s", e.ln.Addr())
		}
		e.dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
			mu.Lock()
			dialled = append(dialled, addr)
			mu.Unlock()
			if strings.HasPrefix(addr, "down.") {
				return nil, errors.New("no route")
			}
			return (&net.Dialer{}).DialContext(ctx, network, echo)
		}
		addr := e.ln.Addr().String()
		for _, target := range []string{"REGISTRY.npmjs.org.:443", "a.b.github.com:443"} {
			status, br, c := egressRequest(t, addr, "CONNECT "+target+" HTTP/1.1", "early;")
			if status != "HTTP/1.1 200 Connection established" {
				t.Fatalf("%s: %q", target, status)
			}
			io.WriteString(c, "late")
			got := make([]byte, len("early;late"))
			if _, err := io.ReadFull(br, got); err != nil || string(got) != "early;late" {
				t.Errorf("%s: echoed %q, %v", target, got, err)
			}
			c.Close()
		}
		for _, line := range []string{
			"CONNECT registry.npmjs.org:80 HTTP/1.1",
			"CONNECT example.com:443 HTTP/1.1",
			"CONNECT github.com:443 HTTP/1.1",
			"CONNECT 127.0.0.1:443 HTTP/1.1",
			"GET http://registry.npmjs.org/ HTTP/1.1",
		} {
			if status, _, c := egressRequest(t, addr, line, ""); !strings.HasPrefix(status, "HTTP/1.1 403") {
				t.Errorf("%s: %q", line, status)
			} else {
				c.Close()
			}
		}
		if status, _, c := egressRequest(t, addr, "CONNECT down.example:443 HTTP/1.1", ""); !strings.HasPrefix(status, "HTTP/1.1 502") {
			t.Errorf("dial failure: %q", status)
		} else {
			c.Close()
		}
		return nil, nil
	})
	if want := []string{"registry.npmjs.org:443", "a.b.github.com:443", "down.example:443"}; !slices.Equal(dialled, want) {
		t.Errorf("dialled %q, want %q", dialled, want)
	}
	evs := telemetry(t, cfg)
	if countEvents(evs, "egress") != 1 || countEvents(evs, "egress-allowed") != 1 || countEvents(evs, "egress-refused") != 1 {
		t.Errorf("egress events: %d start, %d allowed, %d refused", countEvents(evs, "egress"), countEvents(evs, "egress-allowed"), countEvents(evs, "egress-refused"))
	}
	if log := readFile(t, cfg, "log.txt"); !strings.Contains(log, "egress: 3 tunnel(s) allowed, 5 request(s) refused this launch") {
		t.Errorf("no counts in the log:\n%s", log)
	}
}

// close ends an open tunnel: both ends are closed and close returns once
// the copies have ended.
func TestEgressCloseEndsTunnels(t *testing.T) {
	cfg, _ := testConfig(t)
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()
	upConn := make(chan net.Conn, 1)
	go func() {
		c, err := upstream.Accept()
		if err == nil {
			upConn <- c
		}
	}()
	launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.PinHosts([]string{"held.example"})
		e := rt.egress.Load()
		e.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstream.Addr().String())
		}
		status, br, c := egressRequest(t, e.ln.Addr().String(), "CONNECT held.example:443 HTTP/1.1", "")
		if !strings.HasPrefix(status, "HTTP/1.1 200") {
			t.Fatalf("%q", status)
		}
		defer c.Close()
		up := <-upConn // held open: neither side ends the tunnel
		defer up.Close()
		done := make(chan struct{})
		go func() { e.close(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("close did not return with a tunnel open")
		}
		rt.egress.Store(nil) // closed here, not by Run
		if _, err := br.ReadByte(); err == nil {
			t.Error("client end still open")
		}
		up.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := up.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
			t.Errorf("upstream end: %v, want EOF", err)
		}
		if _, err := net.DialTimeout("tcp", e.ln.Addr().String(), time.Second); err == nil {
			t.Error("still accepting after close")
		}
		return nil, nil
	})
}

// No hosts, no egress proxy; the same hosts again do nothing; other hosts
// stop the run.
func TestPinHostsEgress(t *testing.T) {
	cfg, _ := testConfig(t)
	s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.PinHosts(nil)
		if rt.egressPort() != "" {
			t.Error("egress proxy with no hosts")
		}
		rt.PinHosts([]string{})
		if rt.stopped() != nil {
			t.Errorf("an identical empty re-pin stopped: %v", rt.stopped())
		}
		return nil, nil
	})
	if s.StopReason != StopReturned || countEvents(telemetry(t, cfg), "egress") != 0 {
		t.Errorf("%+v", s)
	}

	cfg, _ = testConfig(t)
	s = launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.PinHosts([]string{"a.example"})
		port := rt.egressPort()
		rt.PinHosts([]string{"a.example"})
		if rt.stopped() != nil || rt.egressPort() != port {
			t.Errorf("identical re-pin: stop %v, port %s → %s", rt.stopped(), port, rt.egressPort())
		}
		rt.PinHosts([]string{"b.example"})
		if !slices.Equal(rt.Hosts(), []string{"a.example"}) {
			t.Errorf("hosts %v", rt.Hosts())
		}
		return nil, nil
	})
	if s.StopReason != StopRuntimeError || !strings.Contains(s.Detail, "different list") {
		t.Errorf("different re-pin: %+v", s)
	}
}

// A bind failure stops the run; the hosts stay pinned with no egress port.
func TestEgressBindFailure(t *testing.T) {
	cfg, _ := testConfig(t)
	old := egressListen
	egressListen = "127.0.0.1:99999"
	defer func() { egressListen = old }()
	s := launch(t, cfg, func(ctx context.Context, rt *Runtime) (json.RawMessage, error) {
		rt.PinHosts([]string{"a.example"})
		if rt.egressPort() != "" || len(rt.Hosts()) != 1 {
			t.Errorf("port %q hosts %v", rt.egressPort(), rt.Hosts())
		}
		return nil, nil
	})
	if s.StopReason != StopRuntimeError || !strings.Contains(s.Detail, "egress proxy") {
		t.Errorf("%+v", s)
	}
}
