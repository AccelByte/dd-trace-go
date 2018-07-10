// Package grpc provides functions to trace the google.golang.org/grpc package.
package grpc

import (
	"fmt"
	"strconv"

	"github.com/DataDog/dd-trace-go/tracer"
	"github.com/DataDog/dd-trace-go/tracer/ext"

	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// pass trace ids with these headers
const (
	traceIDKey  = "x-datadog-trace-id"
	parentIDKey = "x-datadog-parent-id"
)

// UnaryServerInterceptor will trace requests to the given grpc server.
func UnaryServerInterceptor(opts ...InterceptorOption) grpc.UnaryServerInterceptor {
	cfg := new(interceptorConfig)
	defaults(cfg)
	for _, fn := range opts {
		fn(cfg)
	}
	if cfg.serviceName == "" {
		cfg.serviceName = "grpc.server"
	}
	t := cfg.tracer
	t.SetServiceInfo(cfg.serviceName, "grpc-server", ext.AppTypeRPC)
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !t.Enabled() {
			return handler(ctx, req)
		}
		span := serverSpan(t, ctx, info.FullMethod, cfg.serviceName)
		resp, err := handler(tracer.ContextWithSpan(ctx, span), req)
		span.FinishWithErr(err)
		return resp, err
	}
}

// UnaryClientInterceptor will add tracing to a gprc client.
func UnaryClientInterceptor(opts ...InterceptorOption) grpc.UnaryClientInterceptor {
	cfg := new(interceptorConfig)
	defaults(cfg)
	for _, fn := range opts {
		fn(cfg)
	}
	if cfg.serviceName == "" {
		cfg.serviceName = "grpc.client"
	}
	t := cfg.tracer
	t.SetServiceInfo(cfg.serviceName, "grpc-client", ext.AppTypeRPC)
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		span, ctx := tracer.StartSpanWithContext(ctx, "grpc.client")
		span.SetMeta("grpc.method", method)
		ctx = setIDs(span, ctx)

		err := invoker(ctx, method, req, reply, cc, opts...)
		span.SetMeta("grpc.code", grpc.Code(err).String())
		span.FinishWithErr(err)
		return err
	}
}

func serverSpan(t *tracer.Tracer, ctx context.Context, method, service string) *tracer.Span {
	span := t.NewRootSpan("grpc.server", service, method)
	span.SetMeta("gprc.method", method)
	span.Type = "go"

	traceID, parentID := getIDs(ctx)
	if traceID != 0 && parentID != 0 {
		span.TraceID = traceID
		span.ParentID = parentID
	}

	return span
}

// setIDs will set the trace ids on the context{
func setIDs(span *tracer.Span, ctx context.Context) context.Context {
	if span == nil || span.TraceID == 0 {
		return ctx
	}
	md := metadata.New(map[string]string{
		traceIDKey:  fmt.Sprint(span.TraceID),
		parentIDKey: fmt.Sprint(span.ParentID),
	})
	if existing, ok := metadata.FromIncomingContext(ctx); ok {
		md = metadata.Join(existing, md)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// getIDs will return ids embededd an ahe context.
func getIDs(ctx context.Context) (traceID, parentID uint64) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if id := getID(md, traceIDKey); id > 0 {
			traceID = id
		}
		if id := getID(md, parentIDKey); id > 0 {
			parentID = id
		}
	}
	return traceID, parentID
}

// getID parses an id from the metadata.
func getID(md metadata.MD, name string) uint64 {
	for _, str := range md[name] {
		id, err := strconv.Atoi(str)
		if err == nil {
			return uint64(id)
		}
	}
	return 0
}
