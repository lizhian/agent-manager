package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/net/http/httpguts"
)

type remoteStatusError struct{ code int }
type remoteConfigError struct{ message string }

func (e *remoteConfigError) Error() string { return e.message }

func (e *remoteStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

// requestTransport binds SDK streams (including detached requests) to this operation's lifetime.
type requestTransport struct {
	base    *http.Transport
	ctx     context.Context
	headers http.Header
	origin  *url.URL
	// The SDK may reduce transport errors to a JSON-RPC code; retain their typed category.
	failureMu sync.Mutex
	failure   error
}

func (t *requestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != t.origin.Scheme || request.URL.Host != t.origin.Host {
		return nil, fmt.Errorf("拒绝跨来源传递 MCP 请求头")
	}
	ctx, cancel := context.WithCancel(request.Context())
	stop := func() bool { return false }
	if request.Method == http.MethodDelete {
		cancel()
		ctx, cancel = context.WithTimeout(request.Context(), time.Second)
	} else {
		stop = context.AfterFunc(t.ctx, cancel)
	}
	clone := request.Clone(ctx)
	for key, values := range t.headers {
		clone.Header[key] = append([]string(nil), values...)
	}
	response, err := t.base.RoundTrip(clone)
	if err != nil {
		stop()
		cancel()
		t.failureMu.Lock()
		t.failure = err
		t.failureMu.Unlock()
		return nil, err
	}
	t.failureMu.Lock()
	t.failure = nil
	t.failureMu.Unlock()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		_ = response.Body.Close()
		stop()
		cancel()
		return nil, &remoteStatusError{response.StatusCode}
	}
	response.Body = &operationBody{ReadCloser: response.Body, release: func() { stop(); cancel() }}
	return response, nil
}

type operationBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *operationBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.release); return err }
func (d *Debugger) connectRemote(ctx context.Context, server Server) (*sdk.ClientSession, func(), error) {
	endpoint, err := url.Parse(server.URL)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, nil, &remoteConfigError{"URL 无效，请填写包含主机名的 http/https 地址"}
	}
	headers := http.Header{}
	env := environmentMap(d.options.Environment())
	for key, value := range server.Headers {
		value = ExpandEnvironment(value, env)
		if !httpguts.ValidHeaderFieldName(key) || !httpguts.ValidHeaderFieldValue(value) {
			return nil, nil, &remoteConfigError{"请求头名称或值格式无效，请检查空格及换行"}
		}
		headers.Set(key, value)
	}
	template, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		template = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	base := template.Clone()
	base.IdleConnTimeout = time.Second
	requests := &requestTransport{base: base, ctx: ctx, headers: headers, origin: endpoint}
	client := &http.Client{Transport: requests, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Configured credentials must not follow redirects to another origin.
		if req.URL.Scheme != endpoint.Scheme || req.URL.Host != endpoint.Host {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return fmt.Errorf("重定向次数过多")
		}
		return nil
	}}
	var transport sdk.Transport
	if server.Transport == "http" {
		transport = &sdk.StreamableClientTransport{Endpoint: server.URL, HTTPClient: client, DisableStandaloneSSE: true, MaxRetries: -1}
	} else {
		transport = &sdk.SSEClientTransport{Endpoint: server.URL, HTTPClient: client}
	}
	session, err := newProtocolClient().Connect(ctx, transport, &sdk.ClientSessionOptions{ProtocolVersion: "2025-06-18"})
	if err != nil {
		base.CloseIdleConnections()
		requests.failureMu.Lock()
		failure := requests.failure
		requests.failureMu.Unlock()
		return nil, nil, errors.Join(err, failure)
	}
	return session, func() { _ = session.Close(); base.CloseIdleConnections() }, nil
}
