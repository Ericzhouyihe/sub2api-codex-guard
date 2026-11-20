package guard

import (
	"crypto/sha256"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxPooledClients = 256

// clientPool 按代理 URL 复用 HTTP Transport。键使用哈希，避免把代理凭据留在 map 键里。
type clientPool struct {
	mu      sync.Mutex
	clients map[[32]byte]*http.Client
}

func newClientPool() *clientPool {
	return &clientPool{clients: make(map[[32]byte]*http.Client)}
}

func proxyKey(proxyURL string) [32]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(proxyURL)))
}

func (p *clientPool) client(proxyURL string) (*http.Client, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	key := proxyKey(proxyURL)
	p.mu.Lock()
	defer p.mu.Unlock()
	if client := p.clients[key]; client != nil {
		return client, nil
	}
	client, err := newHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	if len(p.clients) >= maxPooledClients {
		for old, existing := range p.clients {
			existing.CloseIdleConnections()
			delete(p.clients, old)
		}
	}
	p.clients[key] = client
	return client, nil
}

func (p *clientPool) closeIdle() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, client := range p.clients {
		client.CloseIdleConnections()
	}
}

func newHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 5 * time.Minute,
		ExpectContinueTimeout: time.Second,
		// 透传路径必须原样返回上游字节；由调用方决定 Accept-Encoding。
		DisableCompression: true,
	}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Host == "" {
			return nil, errors.New("invalid proxy URL")
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, errors.New("unsupported proxy scheme")
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &http.Client{
		Transport: transport,
		// 与宿主内置出站行为一致：不跟随重定向，把 3xx 原样交回宿主。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}
