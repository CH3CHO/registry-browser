package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// Kind 是 registry 错误的分类，决定 UI 如何向用户解释问题。
type Kind string

const (
	// KindUnauthorized 表示 401：缺少凭据或凭据失效。
	KindUnauthorized Kind = "unauthorized"
	// KindForbidden 表示 403：已认证但没有该资源的权限。
	KindForbidden Kind = "forbidden"
	// KindNotFound 表示 404：仓库、tag 或镜像不存在。
	KindNotFound Kind = "not_found"
	// KindUnsupported 表示 405/501：该 registry 不实现这个端点。
	KindUnsupported Kind = "unsupported"
	// KindRateLimited 表示 429：被限流。
	KindRateLimited Kind = "rate_limited"
	// KindServerError 表示 5xx：服务端故障。
	KindServerError Kind = "server_error"
	// KindNetwork 表示网络层失败：连接被拒、DNS 失败、超时。
	KindNetwork Kind = "network"
	// KindInvalidResponse 表示响应不是预期的结构。
	KindInvalidResponse Kind = "invalid_response"
	// KindUnknown 表示无法归类。
	KindUnknown Kind = "unknown"
)

// String 返回分类的可读名称。
func (k Kind) String() string { return string(k) }

// Error 是带分类的 registry 错误。
type Error struct {
	// Kind 是错误分类。
	Kind Kind
	// Status 是 HTTP 状态码，网络错误时为 0。
	Status int
	// Op 是失败的操作，例如 "list tags"。
	Op string
	// Message 是可选的补充说明。
	Message string
	// Err 是被包装的原始错误。
	Err error
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	prefix := e.Op
	if prefix == "" {
		prefix = "registry request"
	}
	if e.Status != 0 {
		return fmt.Sprintf("%s failed: %s (HTTP %d): %v", prefix, e.Kind, e.Status, e.Err)
	}
	return fmt.Sprintf("%s failed: %s: %v", prefix, e.Kind, e.Err)
}

// Unwrap 暴露被包装的原始错误。
func (e *Error) Unwrap() error { return e.Err }

// UserHint 返回给用户看的处置建议，没有建议时返回空字符串。
func (e *Error) UserHint() string {
	switch e.Kind {
	case KindUnauthorized, KindForbidden:
		return "凭据缺失或已失效，请先执行 docker login <registry>"
	default:
		return ""
	}
}

// Classify 把底层错误转换成带分类的 *Error。传入 nil 时返回 nil。
func Classify(err error) *Error {
	if err == nil {
		return nil
	}

	var classified *Error
	if errors.As(err, &classified) {
		return classified
	}

	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		return &Error{
			Kind:   kindForStatus(transportErr.StatusCode),
			Status: transportErr.StatusCode,
			Err:    err,
		}
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return &Error{Kind: KindInvalidResponse, Err: err}
	}

	return &Error{Kind: KindNetwork, Err: err}
}

// AsError 把任意错误转换成 *Error，便于调用方读取分类。传入 nil 时返回一个未知分类的错误。
func AsError(err error) *Error {
	if classified := Classify(err); classified != nil {
		return classified
	}
	return &Error{Kind: KindUnknown}
}

// kindForStatus 把 HTTP 状态码映射到错误分类。
func kindForStatus(status int) Kind {
	switch status {
	case http.StatusUnauthorized:
		return KindUnauthorized
	case http.StatusForbidden:
		return KindForbidden
	case http.StatusNotFound:
		return KindNotFound
	case http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return KindUnsupported
	case http.StatusTooManyRequests:
		return KindRateLimited
	}
	if status >= http.StatusInternalServerError {
		return KindServerError
	}
	return KindUnknown
}
