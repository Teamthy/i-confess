package voiceengine

import (
	"net/http"
	"regexp"
)

// Canonical preserves registry/cache names already in use. The trainer and
// licence register use gpt_sovits for the same family; do not guess other aliases.
func (e Engine) Canonical() Engine {
	if e == "gpt_sovits" {
		return EngineGPTSoVITS
	}
	return e
}

// WorkerIdentity is a precondition, not a label to stamp onto returned bytes.
// The registry pin must be independently approved, not learned on first use.
type WorkerIdentity struct {
	Engine   Engine `json:"engine"`
	Revision string `json:"revision"`
}

var engineName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var checkpointRevision = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}\+sha256\.[0-9a-f]{64}$`)

// Valid checks the wire format. Only the non-speech dev engine has no weights.
func (id WorkerIdentity) Valid() bool {
	if id.Engine == EngineDevTone {
		return id.Revision == "dev"
	}
	return engineName.MatchString(string(id.Engine)) && checkpointRevision.MatchString(id.Revision)
}

func (id WorkerIdentity) verify(actual WorkerIdentity) error {
	if !id.Valid() || id.Engine.Canonical() != actual.Engine.Canonical() || id.Revision != actual.Revision {
		// Worker fields are untrusted; never copy their contents into user errors or audit logs.
		return &Error{Class: ClassIdentity, Engine: id.Engine, Msg: "worker engine or checkpoint does not match the model pin"}
	}
	return nil
}

// Reject ambiguous/missing headers, including a legacy combined name-version
// header. Never infer the engine that sent a response from the configured URL.
func responseIdentity(h http.Header) WorkerIdentity {
	if len(h.Values("X-Engine")) != 1 || len(h.Values("X-Engine-Version")) != 1 {
		return WorkerIdentity{}
	}
	return WorkerIdentity{Engine: Engine(h.Get("X-Engine")).Canonical(), Revision: h.Get("X-Engine-Version")}
}
