package push

import (
	"fmt"

	"github.com/Teamthy/i-confess/internal/googleauth"
)

// Google service-account authentication for FCM (§47).
//
// The handshake itself lives in internal/googleauth, because billing uses the
// same flow with a different scope. What stays here is the FCM-specific wiring:
// the scope and the project id derived from the key.
//
// GoogleTokenSource is an alias rather than a wrapper type so the methods
// (Token, SetClock, SetTokenURL, ProjectID) are the shared implementation
// rather than a second copy of them.
type GoogleTokenSource = googleauth.TokenSource

// NewGoogleTokenSource parses a service-account JSON key for FCM.
func NewGoogleTokenSource(keyJSON []byte) (*GoogleTokenSource, error) {
	src, err := googleauth.New(keyJSON, googleauth.ScopeFirebaseMessaging)
	if err != nil {
		return nil, err
	}
	return src, nil
}

// NewFCMFromServiceAccount wires an FCM sender using a service-account key.
func NewFCMFromServiceAccount(keyJSON []byte) (*FCM, error) {
	src, err := NewGoogleTokenSource(keyJSON)
	if err != nil {
		return nil, err
	}
	if src.ProjectID() == "" {
		return nil, fmt.Errorf("service account has no project_id")
	}
	f := NewFCM(src.ProjectID(), src.Token)
	return f, nil
}
