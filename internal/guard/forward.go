package guard

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	pluginv1 "github.com/codexguard/sub2api-codex-guard/internal/pluginapi/v1"
)

const bodyChunkSize = 32 * 1024

// recvFrame 让 Recv 服从 ctx；gRPC 会在 Forward 返回时解除阻塞的 Recv。
func recvFrame(ctx context.Context, stream pluginv1.TransportPlugin_ForwardServer) (*pluginv1.ForwardRequest, error) {
	type result struct {
		frame *pluginv1.ForwardRequest
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		frame, err := stream.Recv()
		ch <- result{frame, err}
	}()
	select {
	case r := <-ch:
		return r.frame, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// pumpBody 把剩余请求帧写入管道，供透传请求流式上传。
func pumpBody(stream pluginv1.TransportPlugin_ForwardServer, writer *io.PipeWriter) {
	for {
		frame, err := stream.Recv()
		if err != nil {
			_ = writer.CloseWithError(errors.New("incomplete request body"))
			return
		}
		switch value := frame.GetFrame().(type) {
		case *pluginv1.ForwardRequest_BodyChunk:
			if _, err := writer.Write(value.BodyChunk); err != nil {
				return
			}
		case *pluginv1.ForwardRequest_BodyEnd:
			if !value.BodyEnd {
				_ = writer.CloseWithError(errors.New("invalid body end frame"))
				return
			}
			_ = writer.Close()
			return
		default:
			_ = writer.CloseWithError(errors.New("unexpected request frame"))
			return
		}
	}
}

func sendError(stream pluginv1.TransportPlugin_ForwardServer, code, message string, sent bool) error {
	return stream.Send(&pluginv1.ForwardResponse{Frame: &pluginv1.ForwardResponse_Error{Error: &pluginv1.ForwardResponseError{
		Code: code, Message: message, RequestSent: sent,
	}}})
}

func toPluginHeaders(headers http.Header) map[string]*pluginv1.HeaderValues {
	out := make(map[string]*pluginv1.HeaderValues, len(headers))
	for key, values := range headers {
		out[key] = &pluginv1.HeaderValues{Values: append([]string(nil), values...)}
	}
	return out
}

func sendEnd(stream pluginv1.TransportPlugin_ForwardServer, received int64, started time.Time) error {
	return stream.Send(&pluginv1.ForwardResponse{Frame: &pluginv1.ForwardResponse_End{End: &pluginv1.ForwardResponseEnd{
		BytesReceived: received, DurationMs: time.Since(started).Milliseconds(),
	}}})
}

// Forward 实现 openai.oauth.outbound_transport.v1：所有请求原样透传，
// 插件只观察/不改写真实 Codex 流量，保证安装后不断流。
func (e *Engine) Forward(stream pluginv1.TransportPlugin_ForwardServer) error {
	started := time.Now()
	ctx := stream.Context()
	if e.closed.Load() {
		return sendError(stream, "plugin_stopped", "插件正在停止", false)
	}
	first, err := recvFrame(ctx, stream)
	if err != nil {
		return sendError(stream, "invalid_request", "missing request start frame", false)
	}
	start := first.GetStart()
	if start == nil {
		return sendError(stream, "invalid_request", "first frame must be request start", false)
	}
	target, err := url.Parse(start.GetUrl())
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" || target.User != nil {
		return sendError(stream, "invalid_request", "invalid upstream URL", false)
	}
	return e.passthrough(ctx, stream, start, started)
}

func (e *Engine) passthrough(ctx context.Context, stream pluginv1.TransportPlugin_ForwardServer, start *pluginv1.ForwardRequestStart, started time.Time) error {
	var body io.Reader
	if start.GetHasBody() {
		reader, writer := io.Pipe()
		defer reader.Close()
		go pumpBody(stream, writer)
		body = reader
	} else {
		body = http.NoBody
	}
	request, err := http.NewRequestWithContext(ctx, start.GetMethod(), start.GetUrl(), body)
	if err != nil {
		return sendError(stream, "invalid_request", "invalid upstream request", false)
	}
	for key, values := range start.GetHeaders() {
		if values != nil {
			request.Header[key] = append([]string(nil), values.GetValues()...)
		}
	}
	if start.GetHost() != "" {
		request.Host = start.GetHost()
	}
	request.ContentLength = start.GetContentLength()
	if !start.GetHasBody() {
		request.ContentLength = 0
		request.Body = http.NoBody
	}
	// 业务 POST 不允许由 Transport 在连接复用失败后自动重放。
	request.GetBody = nil
	client, err := e.clients.client(start.GetProxyUrl())
	if err != nil {
		return sendError(stream, "invalid_proxy", "invalid proxy configuration", false)
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return sendError(stream, "upstream_transport", "upstream transport failed", true)
	}
	defer response.Body.Close()

	if err := stream.Send(&pluginv1.ForwardResponse{Frame: &pluginv1.ForwardResponse_Start{Start: &pluginv1.ForwardResponseStart{
		StatusCode: int32(response.StatusCode), Status: response.Status, Protocol: response.Proto,
		ProtocolMajor: int32(response.ProtoMajor), ProtocolMinor: int32(response.ProtoMinor),
		Headers: toPluginHeaders(response.Header), ContentLength: response.ContentLength,
	}}}); err != nil {
		return err
	}
	buffer := make([]byte, bodyChunkSize)
	var received int64
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if err := stream.Send(&pluginv1.ForwardResponse{Frame: &pluginv1.ForwardResponse_BodyChunk{BodyChunk: append([]byte(nil), buffer[:n]...)}}); err != nil {
				return err
			}
			received += int64(n)
		}
		if readErr == io.EOF {
			return sendEnd(stream, received, started)
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return sendError(stream, "upstream_read", "upstream response was interrupted", true)
		}
	}
}
