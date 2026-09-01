// Package push delivers notifications to iOS and Android devices (§47).
//
// The rule the PRD is explicit about: nothing sends to APNs or FCM directly
// from application code. Callers hand a Notification to a Sender, and this
// package owns credentials, retry classification and token lifecycle. That
// keeps provider keys in one place and makes "stop sending to dead tokens" a
// property of the system rather than something each caller remembers.
package push

import (
	"context"
	"errors"
	"fmt"
)

// Platform identifies a device's push transport.
type Platform string

const (
	PlatformIOS     Platform = "ios"
	PlatformAndroid Platform = "android"
	PlatformWeb     Platform = "web"
)

// Provider names.
const (
	ProviderAPNs = "apns"
	ProviderFCM  = "fcm"
)

// Notification is a message to one device.
type Notification struct {
	Token    string
	Platform Platform
	Title    string
	Body     string
	// Data carries a deep link and context the app uses to open the right
	// screen. Never put anything sensitive here: push payloads traverse
	// Apple's and Google's infrastructure and surface on a lock screen.
	Data map[string]string
	// CollapseKey lets a provider replace an undelivered earlier message with
	// this one. A phone that was offline overnight should get today's
	// reminder, not five stale ones.
	CollapseKey string
	// Sound is the alert sound; empty means the platform default.
	Sound string
}

// Sender delivers a notification. Implementations must be safe for concurrent use.
type Sender interface {
	Name() string
	Send(ctx context.Context, n Notification) error
}

// Delivery outcomes the caller must distinguish.
var (
	// ErrRetryable is a transient fault: timeout, 429, 5xx.
	ErrRetryable = errors.New("retryable push error")
	// ErrPermanent will recur for this payload: malformed request, bad topic.
	ErrPermanent = errors.New("permanent push error")
	// ErrInvalidToken means the device is gone — uninstalled, or the token
	// rotated. The caller must stop sending to it rather than retrying, which
	// is a distinct action from a permanent payload error.
	ErrInvalidToken = errors.New("push token is no longer valid")
)

// Retryable wraps a transient fault.
func Retryable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRetryable, fmt.Sprintf(format, args...))
}

// Permanent wraps a fault that must not be retried.
func Permanent(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPermanent, fmt.Sprintf(format, args...))
}

// InvalidToken wraps a dead-device response.
func InvalidToken(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidToken, fmt.Sprintf(format, args...))
}

// IsRetryable reports whether delivery should be attempted again.
func IsRetryable(err error) bool { return errors.Is(err, ErrRetryable) }

// IsInvalidToken reports whether the token should be purged.
func IsInvalidToken(err error) bool { return errors.Is(err, ErrInvalidToken) }

// Router dispatches to the right provider for a device's platform.
//
// Callers hold a Router rather than a specific Sender so adding a platform
// does not ripple through the scheduler.
type Router struct {
	APNs Sender
	FCM  Sender
}

// Name implements Sender.
func (r *Router) Name() string { return "router" }

// Send implements Sender, choosing a provider by platform.
func (r *Router) Send(ctx context.Context, n Notification) error {
	switch n.Platform {
	case PlatformIOS:
		if r.APNs == nil {
			return Permanent("apns is not configured")
		}
		return r.APNs.Send(ctx, n)
	case PlatformAndroid, PlatformWeb:
		// Web push also goes through FCM, which is what the Firebase web SDK
		// registers against.
		if r.FCM == nil {
			return Permanent("fcm is not configured")
		}
		return r.FCM.Send(ctx, n)
	default:
		return Permanent("unknown platform %q", string(n.Platform))
	}
}

// Configured reports whether any provider is available, so callers can skip
// work rather than generating errors for every device.
func (r *Router) Configured() bool {
	return r != nil && (r.APNs != nil || r.FCM != nil)
}
