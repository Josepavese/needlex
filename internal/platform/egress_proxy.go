package platform

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	egressProxyRequestTimeout = 30 * time.Second
	egressProxyDialTimeout    = 15 * time.Second
	egressProxyIdleTimeout    = 90 * time.Second
)

// egressProxy is a loopback HTTP proxy owned by BrowserRuntime. Chromium sends
// every request to it (--proxy-server with <-loopback> so loopback is not
// implicitly bypassed); it validates CONNECT and absolute-form requests and dials
// the validated address. It is the single enforcement point for autonomous
// browser requests (subresources, fetch/XHR, SSE, WebSocket).
type egressProxy struct {
	egress    *Egress
	listener  net.Listener
	transport *http.Transport
	wg        sync.WaitGroup
	mu        sync.Mutex
	conns     map[net.Conn]struct{}
	closed    bool
}

func startEgressProxy(egress *Egress) (*egressProxy, error) {
	if egress == nil {
		return nil, errors.New("egress proxy requires an enforcing policy")
	}
	listener, err := net.Listen("tcp", cdpLoopbackHost+":0")
	if err != nil {
		return nil, fmt.Errorf("start egress proxy: %w", err)
	}
	proxy := &egressProxy{egress: egress, listener: listener, conns: map[net.Conn]struct{}{}}
	proxy.transport = &http.Transport{
		Proxy:               nil,
		DialContext:         egress.DialContext,
		MaxIdleConns:        8,
		IdleConnTimeout:     egressProxyIdleTimeout,
		TLSHandshakeTimeout: egressProxyDialTimeout,
	}
	proxy.wg.Add(1)
	go func() {
		defer proxy.wg.Done()
		proxy.serve()
	}()
	return proxy, nil
}

func (p *egressProxy) url() string { return "http://" + p.listener.Addr().String() }

// Close stops accepting, closes live tunnels and waits for every owned goroutine.
func (p *egressProxy) Close() error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		_ = p.listener.Close()
		for conn := range p.conns {
			_ = conn.Close()
		}
	}
	p.mu.Unlock()
	p.wg.Wait()
	p.transport.CloseIdleConnections()
	return nil
}

func (p *egressProxy) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		if !p.track(conn) {
			_ = conn.Close()
			continue
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer p.untrack(conn)
			p.handle(conn)
		}()
	}
}

func (p *egressProxy) track(conn net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.conns[conn] = struct{}{}
	return true
}

func (p *egressProxy) untrack(conn net.Conn) {
	p.mu.Lock()
	delete(p.conns, conn)
	p.mu.Unlock()
}

func (p *egressProxy) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(egressProxyRequestTimeout))
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	if request.Method == http.MethodConnect {
		p.tunnel(conn, request)
		return
	}
	p.forward(conn, reader, request)
}

// tunnel validates and dials a CONNECT destination (https and wss).
func (p *egressProxy) tunnel(conn net.Conn, request *http.Request) {
	target, err := connectAddress(request.Host)
	if err != nil {
		writeEgressProxyError(conn, err)
		return
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), egressProxyDialTimeout)
	upstream, err := p.egress.DialContext(dialCtx, "tcp", target)
	cancel()
	if err != nil {
		writeEgressProxyError(conn, err)
		return
	}
	defer upstream.Close()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	proxyTunnel(conn, upstream)
}

// forward validates and forwards an absolute-form plain HTTP request.
func (p *egressProxy) forward(conn net.Conn, reader *bufio.Reader, request *http.Request) {
	if err := p.egress.ValidateURL(request.URL); err != nil {
		writeEgressProxyError(conn, err)
		return
	}
	if !strings.EqualFold(request.URL.Scheme, "http") {
		writeEgressProxyError(conn, &EgressDeniedError{Code: EgressCodeScheme, Host: request.Host, Reason: "proxy forwarding supports only http destinations; https uses CONNECT"})
		return
	}
	outgoing := request.Clone(context.Background())
	outgoing.RequestURI = ""
	outgoing.Header.Del("Proxy-Connection")
	outgoing.Header.Del("Proxy-Authorization")
	if outgoing.URL.Path == "" {
		outgoing.URL.Path = "/"
	}
	response, err := p.transport.RoundTrip(outgoing)
	if err != nil {
		writeEgressProxyError(conn, err)
		return
	}
	if response.StatusCode == http.StatusSwitchingProtocols {
		upstream, ok := response.Body.(io.ReadWriteCloser)
		if !ok {
			writeEgressProxyError(conn, errors.New("upgrade response body is not writable"))
			return
		}
		response.Body = nil // write headers only; the upgrade body is the tunnel
		if err := response.Write(conn); err != nil {
			_ = upstream.Close()
			return
		}
		proxyTunnelBuffer(conn, reader, upstream)
		return
	}
	defer response.Body.Close()
	response.Header.Del("Proxy-Connection")
	_ = response.Write(conn)
}

func connectAddress(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", &EgressDeniedError{Code: EgressCodeHost, Reason: "CONNECT without a destination"}
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host, nil
	}
	return net.JoinHostPort(host, "443"), nil
}

// proxyTunnel copies bytes both ways until either side ends, then half-closes
// the peer so the other copy terminates.
func proxyTunnel(left, right net.Conn) {
	done := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(right, left)
		closeWrite(right)
		done <- struct{}{}
	}()
	_, _ = io.Copy(left, right)
	closeWrite(left)
	<-done
}

func proxyTunnelBuffer(client net.Conn, clientReader *bufio.Reader, upstream io.ReadWriteCloser) {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(upstream, clientReader)
		_ = upstream.Close()
		close(done)
	}()
	_, _ = io.Copy(client, upstream)
	_ = upstream.Close() // server side ended: unblock the client-to-server copy
	<-done
}

func closeWrite(conn net.Conn) {
	if writer, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = writer.CloseWrite()
	}
}

func writeEgressProxyError(conn io.Writer, err error) {
	status, reason := "502 Bad Gateway", err.Error()
	var denied *EgressDeniedError
	if errors.As(err, &denied) {
		status, reason = "403 Forbidden", denied.Error()
	}
	body := reason + "\n"
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 %s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", status, len(body), body)
}
