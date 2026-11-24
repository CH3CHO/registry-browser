package registry

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

func TestClassifyStatusCode(t *testing.T) {
	cases := []struct {
		status int
		want   Kind
	}{
		{http.StatusUnauthorized, KindUnauthorized},
		{http.StatusForbidden, KindForbidden},
		{http.StatusNotFound, KindNotFound},
		{http.StatusMethodNotAllowed, KindUnsupported},
		{http.StatusNotImplemented, KindUnsupported},
		{http.StatusTooManyRequests, KindRateLimited},
		{http.StatusBadGateway, KindServerError},
		{http.StatusServiceUnavailable, KindServerError},
		{http.StatusInternalServerError, KindServerError},
	}

	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			err := Classify(&transport.Error{StatusCode: tc.status})
			if err.Kind != tc.want {
				t.Errorf("Kind = %q, want %q", err.Kind, tc.want)
			}
			if err.Status != tc.status {
				t.Errorf("Status = %d, want %d", err.Status, tc.status)
			}
		})
	}
}

func TestClassifyNonTransportError(t *testing.T) {
	err := Classify(errors.New("dial tcp: connection refused"))
	if err.Kind != KindNetwork {
		t.Errorf("Kind = %q, want %q", err.Kind, KindNetwork)
	}
	if err.Err == nil {
		t.Error("expected the original error to be wrapped")
	}
}

func TestClassifyNil(t *testing.T) {
	if err := Classify(nil); err != nil {
		t.Errorf("Classify(nil) = %v, want nil", err)
	}
}

func TestClassifyAlreadyClassified(t *testing.T) {
	original := &Error{Kind: KindForbidden, Status: http.StatusForbidden}
	if got := Classify(original); got != original {
		t.Errorf("Classify should return the original error, got %v", got)
	}
}

func TestErrorMessage(t *testing.T) {
	err := &Error{Kind: KindUnauthorized, Status: http.StatusUnauthorized, Op: "list tags", Err: errors.New("boom")}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected a non-empty message")
	}
	if !strings.Contains(msg, "list tags") {
		t.Errorf("message %q should mention the operation", msg)
	}
}

func TestErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	err := &Error{Kind: KindNetwork, Err: inner}
	if !errors.Is(err, inner) {
		t.Error("expected errors.Is to find the wrapped error")
	}
}

func TestUserHint(t *testing.T) {
	cases := []struct {
		kind Kind
		want string
	}{
		{KindUnauthorized, "docker login"},
		{KindForbidden, "docker login"},
		{KindNotFound, ""},
		{KindUnsupported, ""},
		{KindRateLimited, ""},
		{KindServerError, ""},
		{KindNetwork, ""},
		{KindUnknown, ""},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			hint := (&Error{Kind: tc.kind}).UserHint()
			if tc.want == "" {
				if hint != "" {
					t.Errorf("UserHint() = %q, want empty", hint)
				}
				return
			}
			if !strings.Contains(hint, tc.want) {
				t.Errorf("UserHint() = %q, want it to contain %q", hint, tc.want)
			}
		})
	}
}

func TestKindString(t *testing.T) {
	if KindUnauthorized.String() != "unauthorized" {
		t.Errorf("String() = %q", KindUnauthorized.String())
	}
}
