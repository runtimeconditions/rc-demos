// Package kratixgate adapts the byte verifier to a Kratix resource workflow.
// It neither validates RC semantics nor evaluates platform support.
package kratixgate

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
	"gopkg.in/yaml.v3"
)

// VerifyRequest unwraps this demo's platform-specific transport, verifies it,
// and returns a request containing only the verified Profile and platform inputs.
// Trust is an operator-supplied key, never a field in the resource request.
func VerifyRequest(data []byte, trusted ed25519.PublicKey) ([]byte, error) {
	var request map[string]any
	if err := yaml.Unmarshal(data, &request); err != nil {
		return nil, &handoff.Failure{Category: "profile_unusable", Detail: err.Error()}
	}
	spec, _ := request["spec"].(map[string]any)
	profile, _ := spec["profile"].(string)
	if _, err := handoff.Envelope([]byte(profile)); err != nil {
		return nil, err
	}
	transport, _ := spec["handoff"].(map[string]any)
	evidence, _ := transport["evidence"].(string)
	signatureText, _ := transport["signature"].(string)
	signature, err := base64.StdEncoding.DecodeString(signatureText)
	if err != nil {
		return nil, &handoff.Failure{Category: "evidence_missing_or_malformed", Detail: "signature must be base64"}
	}
	extensions, ok := transport["extensions"].(map[string]any)
	if !ok {
		return nil, &handoff.Failure{Category: "extension_mismatch", Detail: "extension contents missing"}
	}
	work, err := os.MkdirTemp("", "kratix-handoff-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	if err := os.Mkdir(filepath.Join(work, "extensions"), 0700); err != nil {
		return nil, err
	}
	for digest, raw := range extensions {
		decoded, err := hex.DecodeString(digest)
		content, ok := raw.(string)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != digest || !ok {
			return nil, &handoff.Failure{Category: "extension_mismatch", Detail: "unusable extension transport entry"}
		}
		if err := os.WriteFile(filepath.Join(work, "extensions", digest+".yaml"), []byte(content), 0600); err != nil {
			return nil, err
		}
	}
	for name, content := range map[string][]byte{"profile.yaml": []byte(profile), "evidence.json": []byte(evidence), "evidence.sig": signature} {
		if err := os.WriteFile(filepath.Join(work, name), content, 0600); err != nil {
			return nil, err
		}
	}
	verified, err := handoff.Verify(work, trusted)
	if err != nil {
		return nil, err
	}
	spec["profile"] = string(verified)
	delete(spec, "handoff")
	// JSON is also YAML. This avoids a second YAML interpretation of raw input
	// and preserves the verified Profile string across the Go/Python boundary.
	return json.Marshal(request)
}

// Run publishes to a dedicated shared volume only after verification succeeds.
// The next init container mounts this volume read-only. No unverified fallback.
func Run(input, keyPath, output string) error {
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		return fmt.Errorf("verified output must not already exist: %s", output)
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return &handoff.Failure{Category: "profile_unusable", Detail: err.Error()}
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return &handoff.Failure{Category: "trust_configuration_error", Detail: "cannot read configured public key"}
	}
	verified, err := VerifyRequest(data, ed25519.PublicKey(key))
	if err != nil {
		return err
	}
	return os.WriteFile(output, verified, 0644)
}
