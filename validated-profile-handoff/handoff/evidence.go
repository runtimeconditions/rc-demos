// Package handoff verifies artifact identity and a configured signer's evidence.
// It deliberately has no dependency on the Runtime Conditions validator.
package handoff

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

// These names and fields are private to this experiment, not RC resource types.
const Format = "rc-demos-handoff-experiment-1"
const Validator = "github.com/runtimeconditions/go-rc-profiler/extensioncheck"

var Steps = []string{"ResolveExtensionClosure", "ValidateProfileYAML"}

type Extension struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}
type Evidence struct {
	Format         string      `json:"experiment"`
	ProfileSHA256  string      `json:"profile_sha256"`
	Validator      string      `json:"validator"`
	ProducerSHA256 string      `json:"producer_sha256"`
	Steps          []string    `json:"completed_calls"`
	Extensions     []Extension `json:"resolved_extensions"`
}
type Failure struct{ Category, Detail string }

func (e *Failure) Error() string          { return e.Category + ": " + e.Detail }
func fail(category string, err any) error { return &Failure{category, fmt.Sprint(err)} }
func Digest(data []byte) string           { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func digestOK(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && s == hex.EncodeToString(b)
}

// Envelope checks only what this boundary needs to read; no extension vocabulary,
// interface fields, configuration schemas, or consumer capabilities are checked.
func Envelope(data []byte) ([]string, error) {
	var doc map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&doc); err != nil {
		return nil, fail("profile_unusable", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fail("profile_unusable", "expected one YAML document")
	}
	if doc["apiVersion"] != "runtimeconditions.io/v1alpha1" || doc["kind"] != "RuntimeConditionsProfile" {
		return nil, fail("profile_unusable", "unrecognized Profile envelope")
	}
	conditions, ok := doc["conditions"].([]any)
	if !ok {
		return nil, fail("profile_unusable", "conditions must be a list")
	}
	seen := map[string]bool{}
	for _, c := range conditions {
		m, _ := c.(map[string]any)
		name, _ := m["name"].(string)
		kind, _ := m["kind"].(string)
		if name == "" || kind == "" || seen[name] {
			return nil, fail("profile_unusable", "Conditions need unique names and kinds")
		}
		seen[name] = true
	}
	raw, ok := doc["extensions"].([]any)
	if !ok {
		return nil, fail("profile_unusable", "extensions must be a list")
	}
	ids := make([]string, 0, len(raw))
	seen = map[string]bool{}
	for _, v := range raw {
		id, ok := v.(string)
		if !ok || id == "" || seen[id] {
			return nil, fail("profile_unusable", "extensions need unique nonempty IDs")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

// Verify reads each artifact once. Its returned bytes, not a subsequently reopened
// Profile path, must be passed to the consumer. The public key is caller policy;
// it is never obtained from the untrusted bundle.
func Verify(bundle string, trusted ed25519.PublicKey) ([]byte, error) {
	profile, err := os.ReadFile(filepath.Join(bundle, "profile.yaml"))
	if err != nil {
		return nil, fail("profile_unusable", err)
	}
	ids, err := Envelope(profile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(bundle, "evidence.json"))
	if err != nil {
		return nil, fail("evidence_missing_or_malformed", err)
	}
	var e Evidence
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil {
		return nil, fail("evidence_missing_or_malformed", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fail("evidence_missing_or_malformed", "expected one JSON document")
	}
	if e.Format != Format || e.Validator != Validator || !digestOK(e.ProfileSHA256) || !digestOK(e.ProducerSHA256) || !slices.Equal(e.Steps, Steps) || e.Extensions == nil {
		return nil, fail("evidence_missing_or_malformed", "unrecognized experiment evidence or incomplete validation calls")
	}
	evidenceIDs := make([]string, 0, len(e.Extensions))
	seen := map[string]bool{}
	for _, ext := range e.Extensions {
		if ext.ID == "" || seen[ext.ID] || !digestOK(ext.SHA256) {
			return nil, fail("evidence_missing_or_malformed", "invalid extension entry")
		}
		seen[ext.ID] = true
		evidenceIDs = append(evidenceIDs, ext.ID)
	}
	signature, err := os.ReadFile(filepath.Join(bundle, "evidence.sig"))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, fail("evidence_missing_or_malformed", "missing or unusable detached signature")
	}
	if len(trusted) != ed25519.PublicKeySize || !ed25519.Verify(trusted, data, signature) {
		return nil, fail("evidence_untrusted", "signature does not match configured trusted public key")
	}
	if Digest(profile) != e.ProfileSHA256 {
		return nil, fail("profile_digest_mismatch", "Profile bytes differ from signed evidence")
	}
	slices.Sort(evidenceIDs)
	if !slices.Equal(ids, evidenceIDs) {
		return nil, fail("extension_mismatch", "Profile IDs differ from signed resolved IDs")
	}
	for _, ext := range e.Extensions {
		// Digest syntax was checked before using it as a filename. Contents are opaque
		// here: no dependency resolution, vocabulary checks, or schema validation.
		data, err := os.ReadFile(filepath.Join(bundle, "extensions", ext.SHA256+".yaml"))
		if err != nil || Digest(data) != ext.SHA256 {
			return nil, fail("extension_mismatch", "missing or changed content for "+ext.ID)
		}
	}
	return profile, nil
}
