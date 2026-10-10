package handoff_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/internal/producer"
)

var devBind string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "handoff-consumer-tests-")
	if err != nil {
		panic(err)
	}
	devBind = filepath.Join(dir, "dev-bind")
	cmd := exec.Command("go", "build", "-o", devBind, "./cmd/dev-bind")
	cmd.Dir = "../portable-profile"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func canonical(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../artifacts/request-logger-http.profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func catalogRoot() string {
	if root := os.Getenv("EXTENSIONS_ROOT"); root != "" {
		return root
	}
	return "../../extensions/catalog"
}
func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func setup(t *testing.T, profile []byte) (string, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// This is the real upstream validator, not a stub that signs a valid:true flag.
	a, err := producer.Validate(profile, catalogRoot(), key, handoff.Digest([]byte("test producer")))
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "bundle")
	if err := a.Write(bundle); err != nil {
		t.Fatal(err)
	}
	return bundle, pub, key
}
func evidence(t *testing.T, bundle string) handoff.Evidence {
	t.Helper()
	var e handoff.Evidence
	if err := json.Unmarshal(read(t, filepath.Join(bundle, "evidence.json")), &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRequiredAcceptanceCases(t *testing.T) {
	t.Run("valid_handoff_then_existing_consumer_support_and_fulfillment", func(t *testing.T) {
		original := canonical(t)
		bundle, pub, _ := setup(t, original)
		env := filepath.Join(t.TempDir(), "request-logger.env")
		result := handoff.Consume(bundle, pub, devBind, env)
		if result.Status != "supported" || !result.HandoffVerified || !result.SupportEvaluated {
			t.Fatalf("%+v", result)
		}
		if !bytes.Equal(read(t, filepath.Join(bundle, "profile.yaml")), original) {
			t.Fatal("canonical bytes changed")
		}
		text := string(read(t, env))
		if !strings.Contains(text, "TODOS_API_URL=http://127.0.0.1:18081") || !strings.Contains(text, "REDIS_URL=redis://127.0.0.1:16379") {
			t.Fatal(text)
		}
	})
	cases := []struct {
		name, status string
		change       func(*testing.T, string, ed25519.PrivateKey)
	}{
		{"profile_modified_after_validation", "profile_digest_mismatch", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			p := filepath.Join(b, "profile.yaml")
			write(t, p, append(read(t, p), []byte("\n# byte-only change\n")...))
		}},
		{"different_extension_content", "extension_mismatch", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			e := evidence(t, b)
			p := filepath.Join(b, "extensions", e.Extensions[0].SHA256+".yaml")
			write(t, p, append(read(t, p), []byte("\n# altered definition\n")...))
		}},
		{"untrusted_validation_evidence", "evidence_untrusted", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			_, rogue, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(b, "evidence.sig"), ed25519.Sign(rogue, read(t, filepath.Join(b, "evidence.json"))))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle, pub, key := setup(t, canonical(t))
			tc.change(t, bundle, key)
			assertRejectedBeforeConsumer(t, bundle, pub, tc.status)
		})
	}
	t.Run("valid_but_unsupported_demand", func(t *testing.T) {
		// memcached is explicitly permitted by the real common-integrations extension.
		// It is unsupported by dev-bind, which only provides Redis. Validate the
		// changed Profile upstream before signing it; do not mutate after signing.
		profile := bytes.Replace(canonical(t), []byte("engine: redis"), []byte("engine: memcached"), 1)
		bundle, pub, _ := setup(t, profile)
		env := filepath.Join(t.TempDir(), "must-not-exist.env")
		result := handoff.Consume(bundle, pub, devBind, env)
		if result.Status != "unsupported" || !result.HandoffVerified || !result.SupportEvaluated || !strings.Contains(result.Detail, "memcached") {
			t.Fatalf("%+v", result)
		}
		if _, err := os.Stat(env); !os.IsNotExist(err) {
			t.Fatal("unsupported demand reached fulfillment")
		}
	})
}

func assertRejectedBeforeConsumer(t *testing.T, bundle string, pub ed25519.PublicKey, status string) {
	t.Helper()
	// A sentinel consumer proves the process never starts, independently of the
	// booleans returned by the wrapper. There is no validator in this executable.
	dir := t.TempDir()
	marker := filepath.Join(dir, "called")
	sentinel := filepath.Join(dir, "consumer")
	// Test temp paths are supplied as argv, not shell-interpolated input.
	script := "#!/bin/sh\ntouch \"$(dirname \"$0\")/called\"\nexit 0\n"
	if err := os.WriteFile(sentinel, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	result := handoff.Consume(bundle, pub, sentinel, "")
	if result.Status != status || result.HandoffVerified || result.SupportEvaluated {
		t.Fatalf("want %s before support, got %+v", status, result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("consumer ran before handoff verification")
	}
}

func TestDistinctFailures(t *testing.T) {
	cases := []struct {
		name, status string
		change       func(*testing.T, string, ed25519.PrivateKey)
	}{
		{"malformed_profile", "profile_unusable", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			write(t, filepath.Join(b, "profile.yaml"), []byte("[not: yaml"))
		}},
		{"unusable_envelope", "profile_unusable", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			write(t, filepath.Join(b, "profile.yaml"), []byte("kind: Other\n"))
		}},
		{"multiple_yaml_documents", "profile_unusable", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			p := filepath.Join(b, "profile.yaml")
			write(t, p, append(read(t, p), []byte("\n---\nkind: Other\n")...))
		}},
		{"missing_evidence", "evidence_missing_or_malformed", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			if err := os.Remove(filepath.Join(b, "evidence.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{"malformed_evidence", "evidence_missing_or_malformed", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			write(t, filepath.Join(b, "evidence.json"), []byte("{}"))
		}},
		{"missing_signature", "evidence_missing_or_malformed", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			if err := os.Remove(filepath.Join(b, "evidence.sig")); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing_extension", "extension_mismatch", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			e := evidence(t, b)
			if err := os.Remove(filepath.Join(b, "extensions", e.Extensions[0].SHA256+".yaml")); err != nil {
				t.Fatal(err)
			}
		}},
		{"evidence_tampered", "evidence_untrusted", func(t *testing.T, b string, _ ed25519.PrivateKey) {
			p := filepath.Join(b, "evidence.json")
			write(t, p, append(read(t, p), '\n'))
		}},
		{"signed_but_wrong_extension_ids", "extension_mismatch", func(t *testing.T, b string, key ed25519.PrivateKey) {
			e := evidence(t, b)
			e.Extensions[0].ID += "/wrong"
			data, err := json.Marshal(e)
			if err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(b, "evidence.json"), data)
			write(t, filepath.Join(b, "evidence.sig"), ed25519.Sign(key, data))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle, pub, key := setup(t, canonical(t))
			tc.change(t, bundle, key)
			assertRejectedBeforeConsumer(t, bundle, pub, tc.status)
		})
	}
	t.Run("supported_demand_fulfillment_failure", func(t *testing.T) {
		bundle, pub, _ := setup(t, canonical(t))
		// Existing dev-bind evaluates support successfully, then cannot write to a directory.
		result := handoff.Consume(bundle, pub, devBind, t.TempDir())
		if result.Status != "fulfillment_failed" || !result.HandoffVerified || !result.SupportEvaluated || !strings.Contains(result.Detail, "support: supported") {
			t.Fatalf("%+v", result)
		}
	})
	t.Run("consumer_cannot_start", func(t *testing.T) {
		bundle, pub, _ := setup(t, canonical(t))
		r := handoff.Consume(bundle, pub, filepath.Join(t.TempDir(), "absent"), "")
		if r.Status != "consumer_execution_failed" || !r.HandoffVerified || r.SupportEvaluated {
			t.Fatalf("%+v", r)
		}
	})
}

func TestUpstreamRejectsInvalidDemandWithoutEvidence(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile := bytes.Replace(canonical(t), []byte("engine: redis"), []byte("engine: imaginary"), 1)
	a, err := producer.Validate(profile, catalogRoot(), key, handoff.Digest([]byte("test")))
	if err == nil || a != nil || !strings.Contains(err.Error(), "validation") {
		t.Fatalf("invalid demand produced evidence: %v", err)
	}
}

func TestEvidenceBindsCompleteClosureAndExactBytes(t *testing.T) {
	bundle, pub, _ := setup(t, canonical(t))
	e := evidence(t, bundle)
	if len(e.Extensions) != 2 {
		t.Fatalf("expected common-integrations and env-configuration: %+v", e)
	}
	for _, ext := range e.Extensions {
		data := read(t, filepath.Join(bundle, "extensions", ext.SHA256+".yaml"))
		if handoff.Digest(data) != ext.SHA256 {
			t.Fatal("wrong extension digest")
		}
	}
	verified, err := handoff.Verify(bundle, pub)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(bundle, "profile.yaml"), []byte("changed after verification"))
	if !bytes.Equal(verified, canonical(t)) {
		t.Fatal("verified bytes changed with source path")
	}
}

func TestConsumerDependencyBoundary(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "./cmd/consume").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	for _, forbidden := range []string{"go-rc-profiler", "jsonschema", "/internal/producer"} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("consumer links semantic validator: %s", forbidden)
		}
	}
}
