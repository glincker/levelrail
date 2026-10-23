package trafficexport

import (
	"context"
	"errors"
	"net"
	"net/http"
)

// Error kinds reported in status; they never carry a message, URL or secret.
const (
	KindAuth     = "auth"
	KindRejected = "rejected"
	KindRate     = "rate_limited"
	KindServer   = "server"
	KindBlocked  = "blocked_target"
	KindTimeout  = "timeout"
	KindNetwork  = "network"
	KindConfig   = "config"
	KindOther    = "other"
)

// ErrorKind classifies a delivery error into a short stable label.
func ErrorKind(err error) string {
	if err == nil {
		return ""
	}
	var se *StatusError
	if errors.As(err, &se) {
		switch {
		case se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden:
			return KindAuth
		case se.Code == http.StatusTooManyRequests:
			return KindRate
		case se.Code >= 500:
			return KindServer
		}
		return KindRejected
	}
	var ne net.Error
	switch {
	case errors.Is(err, errBlockedTarget):
		return KindBlocked
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return KindTimeout
	case errors.As(err, &ne):
		return KindNetwork
	}
	return KindOther
}
