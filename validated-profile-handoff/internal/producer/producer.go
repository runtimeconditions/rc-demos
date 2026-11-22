// Package producer is the trusted, upstream side of this experiment. It
// executes the official rc CLI over an exact, private extension snapshot.
// No signing occurs until the CLI accepts those exact bytes.
package producer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
)

type Artifacts struct {
	Profile, Evidence, Signature []byte
	Extensions                   map[string][]byte // content digest -> exact definition bytes
}

func Validate(profile []byte, catalog string, key ed25519.PrivateKey, producerDigest string) (*Artifacts, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("expected an Ed25519 private key")
	}
	profile = append([]byte(nil), profile...)
	ids, err := handoff.Envelope(profile)
	if err != nil {
		return nil, err
	}
	snapshot, err := os.MkdirTemp("", "validation-catalog-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(snapshot)
	definitions, err := resolveSnapshot(ids, catalog, snapshot)
	if err != nil {
		return nil, fmt.Errorf("extension resolution: %w", err)
	}

	// The snapshot contains exactly the transitive published closure, captured
	// once before validation. rc's --catalog mode is offline: it validates these
	// same definition bytes, never a second live download.
	rcPath := os.Getenv("RC_CLI")
	if rcPath == "" {
		rcPath, err = exec.LookPath("rc")
		if err != nil {
			return nil, fmt.Errorf("rc-cli is required (install it or set RC_CLI): %w", err)
		}
	}
	validatorBytes, err := os.ReadFile(rcPath)
	if err != nil {
		return nil, fmt.Errorf("read rc-cli executable: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, rcPath, "profile", "validate", "-", "--catalog", snapshot)
	command.Stdin = bytes.NewReader(profile)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("upstream Profile validation via rc-cli failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), "runtimeconditions: profile validated") {
		return nil, fmt.Errorf("rc-cli did not confirm profile validation: %s", strings.TrimSpace(string(output)))
	}

	evidence := handoff.Evidence{
		Format: handoff.Format, ProfileSHA256: handoff.Digest(profile),
		Validator: handoff.Validator, ValidatorSHA256: handoff.Digest(validatorBytes),
		ProducerSHA256: producerDigest, Steps: append([]string(nil), handoff.Steps...),
		Extensions: []handoff.Extension{},
	}
	artifacts := &Artifacts{Profile: profile, Extensions: map[string][]byte{}}
	orderedIDs := make([]string, 0, len(definitions))
	for id := range definitions {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Strings(orderedIDs)
	for _, id := range orderedIDs {
		data := definitions[id]
		digest := handoff.Digest(data)
		evidence.Extensions = append(evidence.Extensions, handoff.Extension{ID: id, SHA256: digest})
		artifacts.Extensions[digest] = data
	}
	artifacts.Evidence, err = json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return nil, err
	}
	artifacts.Evidence = append(artifacts.Evidence, '\n')
	artifacts.Signature = ed25519.Sign(key, artifacts.Evidence)
	return artifacts, nil
}

// Write publishes a fresh bundle; an old success is never silently reused.
func (a *Artifacts) Write(out string) error {
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		return fmt.Errorf("output must not exist: %s", out)
	}
	dir, err := os.MkdirTemp(filepath.Dir(out), ".handoff-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.Mkdir(filepath.Join(dir, "extensions"), 0700); err != nil {
		return err
	}
	for digest, data := range a.Extensions {
		if err := os.WriteFile(filepath.Join(dir, "extensions", digest+".yaml"), data, 0600); err != nil {
			return err
		}
	}
	for name, data := range map[string][]byte{"profile.yaml": a.Profile, "evidence.json": a.Evidence, "evidence.sig": a.Signature} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
	}
	return os.Rename(dir, out)
}
