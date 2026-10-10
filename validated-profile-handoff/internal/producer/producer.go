// Package producer is the trusted, upstream side of this experiment. Only this
// package imports the RC validator. No signing occurs until both upstream calls
// succeed against private snapshots of the exact bytes recorded in evidence.
package producer

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/runtimeconditions/go-rc-profiler/extensioncheck"
	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
	"gopkg.in/yaml.v3"
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
	// The upstream API accepts directories, not an in-memory catalog. Copy the
	// definitions once, index by metadata.id, and resolve against that private copy.
	snapshot, err := os.MkdirTemp("", "validation-catalog-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(snapshot)
	definitions := map[string][]byte{}
	err = filepath.WalkDir(catalog, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				ID string `yaml:"id"`
			} `yaml:"metadata"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if doc.Kind != "RuntimeConditionsExtensionDefinition" {
			return nil
		}
		if doc.Metadata.ID == "" {
			return fmt.Errorf("%s: missing extension ID", path)
		}
		if _, exists := definitions[doc.Metadata.ID]; exists {
			return fmt.Errorf("duplicate extension ID %s", doc.Metadata.ID)
		}
		definitions[doc.Metadata.ID] = data
		return os.WriteFile(filepath.Join(snapshot, handoff.Digest(data)+".yaml"), data, 0600)
	})
	if err != nil {
		return nil, err
	}
	closure, err := extensioncheck.ResolveExtensionClosure(ids, extensioncheck.ProfileOptions{CatalogRoots: []string{snapshot}})
	if err != nil {
		return nil, fmt.Errorf("extension resolution: %w", err)
	}
	// Validate against only the closure that will cross the boundary. Other catalog
	// entries cannot accidentally contribute vocabulary without being evidenced.
	resolved, err := os.MkdirTemp("", "validation-resolved-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(resolved)
	evidence := handoff.Evidence{Format: handoff.Format, ProfileSHA256: handoff.Digest(profile), Validator: handoff.Validator, ProducerSHA256: producerDigest, Steps: append([]string(nil), handoff.Steps...), Extensions: []handoff.Extension{}}
	artifacts := &Artifacts{Profile: profile, Extensions: map[string][]byte{}}
	for _, id := range closure {
		data, exists := definitions[id]
		if !exists {
			return nil, fmt.Errorf("resolved definition unavailable: %s", id)
		}
		digest := handoff.Digest(data)
		if err := os.WriteFile(filepath.Join(resolved, digest+".yaml"), data, 0600); err != nil {
			return nil, err
		}
		evidence.Extensions = append(evidence.Extensions, handoff.Extension{ID: id, SHA256: digest})
		artifacts.Extensions[digest] = data
	}
	if err := extensioncheck.ValidateProfileYAML(profile, extensioncheck.ProfileOptions{CatalogRoots: []string{resolved}}); err != nil {
		return nil, fmt.Errorf("upstream Profile validation: %w", err)
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
