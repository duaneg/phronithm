package lsfrt

import (
	"encoding/json"
	"errors"
	"slices"
	"time"
)

// class is an attempt's classification (design §2.5).
type class string

const (
	classOK           class = "ok"
	classSkillMissing class = "skill-missing"
	classInvalid      class = "invalid"
	classSessionLimit class = "session-limit"
	classPermanent    class = "permanent"
	classTransient    class = "transient"
	classTimeout      class = "timeout"
	classSpawnError   class = "spawn-error"
	classStopped      class = "stopped"
)

var (
	errGuard  = errors.New("guard timeout")
	errKilled = errors.New("killed by a repeated signal")
)

// Assistant-message error codes that no retry fixes (Claude Code 2.1.289's
// enum; rate_limit, overloaded, server_error and unknown are retryable, and
// max_output_tokens comes with a non-error result, so it classes invalid).
var permanentErrors = []string{
	"authentication_failed", "oauth_org_not_allowed", "account_on_hold", "verification_required",
	"billing_error", "invalid_request", "model_not_found", "cloud_credential_error",
}

var permanentStatuses = []int{400, 401, 403, 404}

// attempt is what one spawn produced.
type attempt struct {
	spawnErr error // the process did not start
	cause    error // context cause when the attempt was cancelled
	exitCode int
	stderr   string // tail
	acc      streamAcc
}

// verdict is an attempt's classification with what the retry loop needs.
type verdict struct {
	class    class
	errors   []string        // schema violations, for invalid
	output   json.RawMessage // the valid structured_output, for ok and skill-missing
	resetsAt time.Time       // for session-limit; zero when the stream gave none
	// guardKilled marks an ok or skill-missing verdict whose process the
	// guard timeout killed after its result: the work is done, the exit hung.
	guardKilled bool
	// auth marks a permanent verdict that is an authentication failure (a
	// 401, or authentication_failed): the credential, not the call, failed.
	auth bool
}

// classify applies design §2.5. The result event decides first: the CLI can
// recover from a mid-stream assistant error and still return a valid result.
// A cancellation other than errGuard or errKilled (the parent context) is the
// caller's to check before classifying.
func classify(a *attempt, schema json.RawMessage, skill string, prefixed bool) verdict {
	switch {
	case errors.Is(a.cause, errKilled):
		return verdict{class: classStopped}
	case errors.Is(a.cause, errGuard):
		if v := classifyResult(a, schema, skill, prefixed); v.class == classOK || v.class == classSkillMissing {
			v.guardKilled = true
			return v
		}
		// Killed while the CLI retried a 401, or after it gave up: the
		// credential failed, not the call. A retry counts only here, where
		// no outcome followed it.
		if authFailed(&a.acc) || a.acc.AuthRetries > 0 {
			return verdict{class: classPermanent, auth: true}
		}
		return verdict{class: classTimeout}
	case a.spawnErr != nil:
		return verdict{class: classSpawnError, errors: []string{a.spawnErr.Error()}}
	}
	if v := classifyResult(a, schema, skill, prefixed); v.class != "" {
		return v
	}
	r := a.acc.Result
	if slices.Contains(a.acc.AssistantErrors, "rate_limit") {
		v := verdict{class: classSessionLimit}
		if a.acc.RejectedResetsAt > 0 {
			v.resetsAt = time.Unix(a.acc.RejectedResetsAt, 0)
		}
		return v
	}
	if slices.ContainsFunc(a.acc.AssistantErrors, func(e string) bool { return slices.Contains(permanentErrors, e) }) ||
		(r != nil && slices.Contains(permanentStatuses, r.APIErrorStatus)) {
		return verdict{class: classPermanent, auth: authFailed(&a.acc)}
	}
	return verdict{class: classTransient}
}

// authFailed reports a stream whose outcome is a failed authentication: an
// authentication_failed assistant error or a 401 result. A 401 the CLI
// retried and then got past is not one.
func authFailed(acc *streamAcc) bool {
	return slices.Contains(acc.AssistantErrors, "authentication_failed") || acc.Result != nil && acc.Result.APIErrorStatus == 401
}

// classifyResult classifies a clean result event: ok, skill-missing or
// invalid. It returns the zero verdict when there is no clean result.
func classifyResult(a *attempt, schema json.RawMessage, skill string, prefixed bool) verdict {
	r := a.acc.Result
	if r == nil || r.IsError {
		return verdict{}
	}
	if len(r.StructuredOutput) == 0 || string(r.StructuredOutput) == "null" {
		return verdict{class: classInvalid, errors: []string{NoOutput}}
	}
	if errs := validateOutput(schema, r.StructuredOutput); len(errs) > 0 {
		return verdict{class: classInvalid, errors: errs}
	}
	if skill != "" && !a.acc.skillLoaded(skill, prefixed) {
		return verdict{class: classSkillMissing, output: r.StructuredOutput}
	}
	return verdict{class: classOK, output: r.StructuredOutput}
}
