package lsfrt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The egress proxy (docs/design/lsf-runtime.md, Permissions → Network): with
// hosts pinned, it listens on 127.0.0.1 at an ephemeral port for the rest of
// the launch, spliced into every agent call's container and setup's. It
// allows CONNECT to port 443 of a pinned host and refuses everything else,
// plain HTTP included. Names are resolved on the host, after matching.

// egressListen is the egress proxy's listen address; tests replace it.
var egressListen = "127.0.0.1:0"

const (
	egressTargetPort  = "443"
	egressDialTimeout = 30 * time.Second
	egressRefusal     = "lsf-egress: only CONNECT to port 443 of a declared host\n"
)

type egressProxy struct {
	rt    *Runtime
	ln    net.Listener
	srv   *http.Server
	done  chan struct{}
	allow map[string]bool // normalised entries
	dial  func(ctx context.Context, network, addr string) (net.Conn, error)
	// onDecision, when set (tests), sees every target and whether it was
	// allowed.
	onDecision func(target string, allowed bool)

	mu               sync.Mutex
	closed           bool
	conns            map[net.Conn]struct{} // both ends of every open tunnel
	wg               sync.WaitGroup        // open handlers and tunnels
	allows, refusals int
}

// normaliseHost is a host or entry as matched: brackets stripped from an
// IPv6 literal, lower-cased, one trailing dot trimmed.
func normaliseHost(h string) string {
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	return strings.TrimSuffix(strings.ToLower(h), ".")
}

// startEgress starts the egress proxy for hosts.
func (rt *Runtime) startEgress(hosts []string) (*egressProxy, error) {
	e := &egressProxy{rt: rt, done: make(chan struct{}), allow: map[string]bool{}, conns: map[net.Conn]struct{}{},
		dial: (&net.Dialer{}).DialContext, onDecision: rt.egressHook}
	for _, h := range hosts {
		e.allow[normaliseHost(h)] = true
	}
	ln, err := net.Listen("tcp", egressListen)
	if err != nil {
		return nil, err
	}
	e.ln = ln
	e.srv = &http.Server{Handler: e, ReadHeaderTimeout: 30 * time.Second, ErrorLog: log.New(logWriter{rt}, "egress: ", 0)}
	go func() {
		defer close(e.done)
		if err := e.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			rt.Log("egress: " + err.Error())
		}
	}()
	rt.Log(fmt.Sprintf("egress: listening on %s for %d host(s)", ln.Addr(), len(e.allow)))
	rt.event("egress", map[string]any{"listen": ln.Addr().String(), "hosts": len(e.allow)})
	return e, nil
}

// allowed reports whether a CONNECT target (host:port) may be tunnelled, and
// its normalised host. The port must be 443. An IP literal matches only the
// identical entry; a name matches an entry exactly, or a "*." entry when
// it ends with the entry's ".suffix" and has at least one label before it.
func (e *egressProxy) allowed(target string) (string, bool) {
	h, port, err := net.SplitHostPort(target)
	if err != nil || port != egressTargetPort {
		return "", false
	}
	host := normaliseHost(h)
	// No wildcard, zone or empty label in a target. The bracket checks
	// repeat SplitHostPort's own.
	if strings.HasPrefix(target, "[") && !strings.Contains(host, ":") {
		return "", false
	}
	if host == "" || strings.ContainsAny(host, "*%[]") || strings.HasPrefix(host, ".") || strings.Contains(host, "..") || strings.HasSuffix(host, ".") {
		return "", false
	}
	if e.allow[host] {
		return host, true
	}
	if net.ParseIP(host) != nil {
		return "", false
	}
	for entry := range e.allow {
		suffix, ok := strings.CutPrefix(entry, "*")
		if !ok || !strings.HasPrefix(suffix, ".") {
			continue
		}
		if prefix, ok := strings.CutSuffix(host, suffix); ok && prefix != "" {
			return host, true
		}
	}
	return "", false
}

func (e *egressProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A handler counts as open until it has handed its connection to a
	// tunnel, so close waits for it.
	if !e.enter() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	defer e.wg.Done()
	// For CONNECT, Go's server takes r.Host from the request-target's
	// authority, never the Host header.
	target := r.Host
	host, ok := "", false
	if r.Method == http.MethodConnect {
		host, ok = e.allowed(target)
	}
	e.decide(r.Method, target, ok)
	if !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, egressRefusal)
		return
	}
	dctx, cancel := context.WithTimeout(r.Context(), egressDialTimeout)
	up, err := e.dial(dctx, "tcp", net.JoinHostPort(host, egressTargetPort))
	cancel()
	if err != nil {
		e.rt.Log(fmt.Sprintf("egress: %s: dial: %v", target, err))
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	client, brw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		up.Close()
		e.rt.Log(fmt.Sprintf("egress: %s: hijack: %v", target, err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// What the client sent after the CONNECT's headers, read with them.
	var early []byte
	if n := brw.Reader.Buffered(); n > 0 {
		b, _ := brw.Reader.Peek(n)
		early = append(early, b...)
	}
	client.SetDeadline(time.Time{})
	if !e.track(client, up) {
		client.Close()
		up.Close()
		return
	}
	go e.tunnel(client, up, early)
}

// enter counts a handler in, false once the proxy is closing.
func (e *egressProxy) enter() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return false
	}
	e.wg.Add(1)
	return true
}

// tunnel answers the CONNECT and copies both ways, half-closing each
// direction as it ends at EOF; both ends close when both directions have,
// or at once when either fails.
func (e *egressProxy) tunnel(client, up net.Conn, early []byte) {
	defer e.wg.Done()
	defer e.untrack(client, up)
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}
	var inner sync.WaitGroup
	inner.Go(func() {
		if _, err := up.Write(early); err != nil {
			client.Close()
			up.Close()
			return
		}
		e.pipe(up, client)
	})
	e.pipe(client, up)
	inner.Wait()
}

// pipe copies src to dst: at EOF it half-closes dst; on an error it closes
// both, which ends the other direction too.
func (e *egressProxy) pipe(dst, src net.Conn) {
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		src.Close()
		return
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	} else {
		dst.Close()
	}
}

// track records a tunnel's ends, false once the proxy is closing.
func (e *egressProxy) track(conns ...net.Conn) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return false
	}
	for _, c := range conns {
		e.conns[c] = struct{}{}
	}
	e.wg.Add(1)
	return true
}

func (e *egressProxy) untrack(conns ...net.Conn) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range conns {
		c.Close()
		delete(e.conns, c)
	}
}

// decide counts a decision; the first allow and the first refusal of the
// launch are logged and emitted.
func (e *egressProxy) decide(method, target string, ok bool) {
	if e.onDecision != nil {
		e.onDecision(target, ok)
	}
	e.mu.Lock()
	first := false
	kind := "egress-refused"
	if ok {
		kind = "egress-allowed"
		e.allows++
		first = e.allows == 1
	} else {
		e.refusals++
		first = e.refusals == 1
	}
	e.mu.Unlock()
	if first {
		e.rt.Log(fmt.Sprintf("egress: %s %q %q (logged once per launch)", strings.TrimPrefix(kind, "egress-"), method, target))
		e.rt.event(kind, map[string]any{"method": method, "target": target})
	}
}

// close stops the proxy at launch end: no new connections, every tunnel's
// ends closed, and every handler and copy ended.
func (e *egressProxy) close() {
	e.srv.Close()
	e.mu.Lock()
	e.closed = true
	for c := range e.conns {
		c.Close()
	}
	allows, refusals := e.allows, e.refusals
	e.mu.Unlock()
	e.wg.Wait()
	<-e.done
	e.rt.Log(fmt.Sprintf("egress: %d request(s) allowed, %d refused this launch", allows, refusals))
}

// port is the port the egress proxy is bound to.
func (e *egressProxy) port() string {
	_, port, _ := net.SplitHostPort(e.ln.Addr().String())
	return port
}

// egressPort is the running egress proxy's port, "" when there is none.
func (rt *Runtime) egressPort() string {
	if e := rt.egress.Load(); e != nil {
		return e.port()
	}
	return ""
}
