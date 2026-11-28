package platform

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Error codes the platform uses in a GraphQL error's extensions.code.
const (
	CodeNotFound       = "NOT_FOUND"
	CodeConflict       = "CONFLICT"
	CodeValidation     = "VALIDATION_ERROR"
	CodeQuotaExceeded  = "QUOTA_EXCEEDED"
	CodeForbidden      = "FORBIDDEN"
	CodeAuthorization  = "AUTHORIZATION_ERROR"
	CodeAuthentication = "AUTHENTICATION_ERROR"
	CodeNotConfigured  = "NOT_CONFIGURED"
	// CodeVerificationFailed is VerifyDomain not finding the TXT record yet.
	CodeVerificationFailed = "VERIFICATION_FAILED"
)

// GraphQLError is one entry of a GraphQL errors array.
type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// Error is what the API answered when it refused an operation: GraphQL
// errors, or an error status with a message.
type Error struct {
	Errors []GraphQLError
	// StatusCode is the HTTP status of the answer.
	StatusCode int
	// Message is set when the answer carried no GraphQL errors.
	Message string
	code    string
}

// FieldError is what the platform found wrong with one field of an input.
type FieldError struct {
	// Field is the name of the input's field, for example "ttl".
	Field   string
	Message string
	// Code is the platform's reason, for example INVALID.
	Code string
}

// Fields lists the fields the platform refused, in the order it named them.
// A refused input usually carries its explanation here and only a general
// text in the message.
func (e *Error) Fields() []FieldError {
	var out []FieldError
	for _, ge := range e.Errors {
		out = append(out, ge.fields()...)
	}
	return out
}

func (ge GraphQLError) fields() []FieldError {
	list, _ := ge.Extensions["fields"].([]any)
	var out []FieldError
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		f := FieldError{}
		f.Field, _ = entry["field"].(string)
		f.Message, _ = entry["message"].(string)
		f.Code, _ = entry["code"].(string)
		if f.Field != "" || f.Message != "" {
			out = append(out, f)
		}
	}
	return out
}

// Error implements error. The fields the platform refused follow the message
// they came with.
func (e *Error) Error() string {
	if len(e.Errors) == 0 {
		return "platform: " + e.Message
	}
	parts := make([]string, 0, len(e.Errors))
	for _, ge := range e.Errors {
		part := ge.Message
		if code, ok := ge.Extensions["code"].(string); ok && code != "" {
			part = fmt.Sprintf("%s (%s)", ge.Message, code)
		}
		for _, f := range ge.fields() {
			text := f.Message
			if text == "" {
				text = f.Code
			}
			part += fmt.Sprintf(": %s: %s", f.Field, text)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

// Code returns the first error's extensions.code, for example NOT_FOUND.
func (e *Error) Code() string {
	if e.code != "" {
		return e.code
	}
	for _, ge := range e.Errors {
		if code, ok := ge.Extensions["code"].(string); ok {
			return code
		}
	}
	return ""
}

// HasCode reports whether err is an API error with the given code.
func HasCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code() == code
}

// Kind sorts errors into the few classes a caller acts on differently.
type Kind int

// The kinds of errors.
const (
	KindOther Kind = iota
	KindUnauthenticated
	KindForbidden
	KindNotFound
	KindConflict
	KindQuota
	KindInvalid
	KindUnavailable
)

// KindOf classifies any error. Errors that did not come from the platform
// are KindOther, except ErrNotSignedIn and ErrSessionExpired.
func KindOf(err error) Kind {
	if errors.Is(err, ErrNotSignedIn) || errors.Is(err, ErrSessionExpired) {
		return KindUnauthenticated
	}
	var data *DataError
	if errors.As(err, &data) {
		return KindOfCode(data.Code, data.StatusCode)
	}
	var e *Error
	if !errors.As(err, &e) {
		return KindOther
	}
	return KindOfCode(e.Code(), e.StatusCode)
}

// KindOfCode classifies an answer by the platform's error code and, when the
// code says nothing, by the HTTP status.
func KindOfCode(code string, status int) Kind {
	switch strings.ToUpper(code) {
	case "AUTHENTICATION_ERROR", "UNAUTHENTICATED":
		return KindUnauthenticated
	case "FORBIDDEN", "PERMISSION_DENIED", "AUTHORIZATION_ERROR":
		return KindForbidden
	case "NOT_FOUND":
		return KindNotFound
	case "CONFLICT", "DUPLICATE_ENTRY":
		return KindConflict
	case "QUOTA_EXCEEDED", "INSUFFICIENT_QUOTA", "LIMIT_EXCEEDED":
		return KindQuota
	case "VALIDATION_ERROR", "BAD_USER_INPUT", "GRAPHQL_VALIDATION_FAILED":
		return KindInvalid
	case "NOT_CONFIGURED", "NOT_READY", "UPSTREAM_ERROR":
		return KindUnavailable
	}
	switch status {
	case http.StatusUnauthorized:
		return KindUnauthenticated
	case http.StatusForbidden:
		return KindForbidden
	case http.StatusNotFound:
		return KindNotFound
	case http.StatusConflict:
		return KindConflict
	case http.StatusTooManyRequests:
		return KindQuota
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusRequestEntityTooLarge:
		return KindInvalid
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return KindUnavailable
	}
	return KindOther
}

// IsNotFound reports whether err says the object does not exist. Some
// operations pass a "not found" through as text; those count too.
func IsNotFound(err error) bool {
	if KindOf(err) == KindNotFound {
		return true
	}
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	msg := strings.ToLower(e.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "404")
}

// IsConflict reports whether err says the object already exists.
func IsConflict(err error) bool { return KindOf(err) == KindConflict }

// IsQuotaExceeded reports whether err says the tenant's quota does not allow the operation.
func IsQuotaExceeded(err error) bool { return KindOf(err) == KindQuota }

// IsForbidden reports whether err says the caller may not do this.
func IsForbidden(err error) bool { return KindOf(err) == KindForbidden }

// IsUnauthenticated reports whether err says the caller is not signed in.
func IsUnauthenticated(err error) bool { return KindOf(err) == KindUnauthenticated }

// errNoTenant is returned by the operations that cannot work without a tenant.
var errNoTenant = errors.New("platform: this operation needs a tenant (WithTenant)")

// notFound builds the error for a lookup the API answered with nothing.
func notFound(what, name string) error {
	return &Error{StatusCode: http.StatusOK, Message: fmt.Sprintf("%s %q not found", what, name), code: CodeNotFound}
}
