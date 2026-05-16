package audit

import "context"

type requestInfoKey struct{}

// RequestInfo holds per-request metadata captured once by
// RequestContextMiddleware and reused by every audit event of the request
// so records from different components can be joined on RequestID.
type RequestInfo struct {
	RequestID string
	IP        string
	UserAgent string
	Method    string
	Path      string
}

func ContextWithRequestInfo(ctx context.Context, info *RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey{}, info)
}

func RequestInfoFromContext(ctx context.Context) *RequestInfo {
	info, _ := ctx.Value(requestInfoKey{}).(*RequestInfo)

	return info
}

// RequestIDFromContext returns the correlation ID assigned to the request,
// or "" when the request did not pass through RequestContextMiddleware.
func RequestIDFromContext(ctx context.Context) string {
	if info := RequestInfoFromContext(ctx); info != nil {
		return info.RequestID
	}

	return ""
}
