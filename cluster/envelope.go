package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

const (
	HeaderFrom           = "Callfs-From"
	HeaderOnBehalfOf     = "Callfs-On-Behalf-Of"
	HeaderDeadline       = "Callfs-Deadline"
	HeaderStatus         = "Callfs-Status"
	HeaderErrorCode      = "Callfs-Error-Code"
	HeaderError          = "Callfs-Error"
	HeaderIdempotencyKey = "Callfs-Idempotency-Key"
	HeaderRetryAfter     = "Retry-After"

	internalProxyUserID = "internal-proxy"
	maxOnBehalfLen      = 256
)

type Error struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration
}

func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func (e *Error) Error() string {
	if "" == e.Message {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Is(target error) bool {
	switch target {
	case context.DeadlineExceeded:
		return ErrTimeout.Code == e.Code
	case context.Canceled:
		return ErrCanceled.Code == e.Code
	}
	other, ok := target.(*Error)
	return ok && other.Code == e.Code
}

var (
	ErrNoResponders    = &Error{Status: http.StatusServiceUnavailable, Code: "no_responders", Message: "no node serves this request"}
	ErrBusy            = &Error{Status: http.StatusServiceUnavailable, Code: "busy", Message: "service busy", RetryAfter: time.Second}
	ErrQueueFull       = &Error{Status: http.StatusServiceUnavailable, Code: "queue_full", Message: "task queue full", RetryAfter: 5 * time.Second}
	ErrClosed          = &Error{Status: http.StatusServiceUnavailable, Code: "closed", Message: "bus closed"}
	ErrTimeout         = &Error{Status: http.StatusGatewayTimeout, Code: "timeout", Message: "request timed out"}
	ErrCanceled        = &Error{Status: 499, Code: "canceled", Message: "request canceled"}
	ErrInvalidName     = &Error{Status: http.StatusBadRequest, Code: "invalid_name", Message: "invalid service, method, topic or pool name"}
	ErrInvalidOnBehalf = &Error{Status: http.StatusBadRequest, Code: "invalid_on_behalf", Message: "invalid on-behalf-of user"}
	ErrInvalidTask     = &Error{Status: http.StatusBadRequest, Code: "invalid_task", Message: "task id is required"}
	ErrInvalidHandler  = &Error{Status: http.StatusBadRequest, Code: "invalid_handler", Message: "handler is required"}
	ErrInvalidTTL      = &Error{Status: http.StatusBadRequest, Code: "invalid_ttl", Message: "ttl must be positive"}
	ErrAlreadyServing  = &Error{Status: http.StatusConflict, Code: "already_serving", Message: "service already registered"}
)

type errorBody struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	RetryAfterMS int64  `json:"retry_after_ms,omitempty"`
}

func ToError(err error) *Error {
	if nil == err {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrCanceled
	}
	return &Error{Status: http.StatusInternalServerError, Code: "internal", Message: err.Error()}
}

func EncodeError(err error) (int, string, []byte) {
	e := ToError(err)
	if nil == e {
		return http.StatusOK, "", nil
	}
	status := e.Status
	if 400 > status {
		status = http.StatusInternalServerError
	}
	body, marshalErr := json.Marshal(errorBody{Code: e.Code, Message: e.Message, RetryAfterMS: e.RetryAfter.Milliseconds()})
	if nil != marshalErr {
		return status, e.Code, nil
	}
	return status, e.Code, body
}

func DecodeError(status int, code string, body []byte) error {
	if 400 > status {
		return nil
	}
	parsed := errorBody{Code: code}
	if 0 != len(body) {
		if err := json.Unmarshal(body, &parsed); nil != err {
			parsed.Message = string(body)
		}
	}
	if "" == parsed.Code {
		parsed.Code = code
	}
	if "" == parsed.Code {
		parsed.Code = "internal"
	}
	if "" == parsed.Message {
		parsed.Message = http.StatusText(status)
	}
	return &Error{
		Status:     status,
		Code:       parsed.Code,
		Message:    parsed.Message,
		RetryAfter: time.Duration(parsed.RetryAfterMS) * time.Millisecond,
	}
}

func StatusOf(err error) int {
	if nil == err {
		return http.StatusOK
	}
	return ToError(err).Status
}

func FormatStatus(status int) string {
	return strconv.Itoa(status)
}

func ParseStatus(s string) (int, error) {
	if "" == s {
		return http.StatusOK, nil
	}
	status, err := strconv.Atoi(s)
	if nil != err {
		return 0, err
	}
	if 100 > status || 599 < status {
		return 0, errors.New("status out of range: " + s)
	}
	return status, nil
}

func FormatRetryAfter(d time.Duration) string {
	seconds := int64((d + time.Second - 1) / time.Second)
	return strconv.FormatInt(max(seconds, 1), 10)
}

func FormatDeadline(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return strconv.FormatInt(t.UnixNano(), 10)
}

func ParseDeadline(s string) (time.Time, error) {
	if "" == s {
		return time.Time{}, nil
	}
	nanos, err := strconv.ParseInt(s, 10, 64)
	if nil != err {
		return time.Time{}, err
	}
	return time.Unix(0, nanos), nil
}

func ValidOnBehalfOf(user string) bool {
	if maxOnBehalfLen < len(user) || internalProxyUserID == user {
		return false
	}
	for _, r := range user {
		if ' ' >= r || 0x7f == r {
			return false
		}
	}
	return true
}

func Encode(v any) ([]byte, error) {
	if nil == v {
		return nil, nil
	}
	return json.Marshal(v)
}

func Decode(raw []byte, v any) error {
	if nil == v || 0 == len(raw) {
		return nil
	}
	return json.Unmarshal(raw, v)
}
