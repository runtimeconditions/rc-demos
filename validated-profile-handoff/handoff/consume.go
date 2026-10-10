package handoff

import (
	"crypto/ed25519"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// Result makes the rejecting boundary visible without interpreting RC semantics.
type Result struct {
	Status           string `json:"status"`
	HandoffVerified  bool   `json:"handoff_verified"`
	SupportEvaluated bool   `json:"support_evaluated"`
	Detail           string `json:"detail"`
}

// Consume wraps the existing dev-bind executable unchanged. Its exit codes are
// the consumer contract: 2 unreadable Profile, 3 unsupported demand, 1 failure.
// In this adapter fulfillment means writing the development environment file;
// resource startup and application health remain the existing demo's concern.
func Consume(bundle string, trusted ed25519.PublicKey, devBind, envOut string) Result {
	profile, err := Verify(bundle, trusted)
	if err != nil {
		var failure *Failure
		if errors.As(err, &failure) {
			return Result{Status: failure.Category, Detail: failure.Detail}
		}
		return Result{Status: "handoff_failure", Detail: err.Error()}
	}
	r := Result{HandoffVerified: true}
	work, err := os.MkdirTemp("", "verified-profile-")
	if err != nil {
		r.Status = "consumer_execution_failed"
		r.Detail = err.Error()
		return r
	}
	defer os.RemoveAll(work)
	path := filepath.Join(work, "profile.yaml")
	if err := os.WriteFile(path, profile, 0600); err != nil {
		r.Status = "consumer_execution_failed"
		r.Detail = err.Error()
		return r
	}
	args := []string{"-profile", path}
	if envOut != "" {
		args = append(args, "-env-out", envOut)
	}
	output, err := exec.Command(devBind, args...).CombinedOutput()
	r.Detail = string(output)
	if err == nil {
		r.Status = "supported"
		r.SupportEvaluated = true
		return r
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		r.Status = "consumer_execution_failed"
		r.Detail = err.Error()
		return r
	}
	switch exit.ExitCode() {
	case 2:
		r.Status = "profile_unusable"
	case 3:
		r.Status = "unsupported"
		r.SupportEvaluated = true
	case 1:
		r.Status = "fulfillment_failed"
		r.SupportEvaluated = true
	default:
		r.Status = "consumer_execution_failed"
	}
	return r
}
