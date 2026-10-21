package producer

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/runtimeconditions/go-rc-profiler/extensioncheck"
	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
	resolver "github.com/runtimeconditions/rc-extension-resolver"
)

const commonID = "https://runtimeconditions.io/extensions/common-integrations/v1alpha1/runtimeconditions.extension.yaml"
const envID = "https://runtimeconditions.io/extensions/env-configuration/v1alpha1/runtimeconditions.extension.yaml"

func publishedFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	docs := map[string][]byte{}
	for id, name := range map[string]string{commonID: "common.yaml", envID: "env.yaml"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		docs[id] = data
	}
	return docs
}

func TestCatalogClosureSnapshotsExactBytesOnce(t *testing.T) {
	docs := publishedFixtures(t)
	docs[commonID] = append(docs[commonID], []byte("\n# exact-byte fixture\n")...)
	count := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := "https://runtimeconditions.io" + r.URL.Path
		count[id]++
		if data, ok := docs[id]; ok {
			w.Write(data)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	httpLoader := resolver.NewHTTPLoader(time.Second)
	loader := func(id string) (io.ReadCloser, error) {
		return httpLoader(server.URL + strings.TrimPrefix(id, "https://runtimeconditions.io"))
	}
	snapshot := t.TempDir()
	got, err := snapshotWithLoader([]string{envID, commonID}, loader, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("closure: %v", got)
	}
	for id, data := range docs {
		if count[id] != 1 {
			t.Fatalf("%s fetched %d times", id, count[id])
		}
		saved, err := os.ReadFile(filepath.Join(snapshot, handoff.Digest(data)+".yaml"))
		if err != nil || !bytes.Equal(saved, data) || !bytes.Equal(got[id], data) {
			t.Fatalf("bytes changed: %s: %v", id, err)
		}
	}
}

func TestCatalogRetrievalFailsClosed(t *testing.T) {
	for _, status := range []int{404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			get := resolver.NewHTTPLoader(time.Second)
			snapshot := t.TempDir()
			_, err := snapshotWithLoader([]string{envID}, func(string) (io.ReadCloser, error) { return get(server.URL) }, snapshot)
			if err == nil {
				t.Fatal("retrieval failure accepted")
			}
			entries, _ := os.ReadDir(snapshot)
			if len(entries) != 0 {
				t.Fatal("partial snapshot published")
			}
		})
	}
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	_, err := snapshotWithLoader([]string{envID}, func(string) (io.ReadCloser, error) { return resolver.NewHTTPLoader(time.Second)(server.URL) }, t.TempDir())
	if err == nil {
		t.Fatal("connection failure accepted")
	}
}

func TestCatalogRejectsMissingDependencyInvalidAndConflictingDocuments(t *testing.T) {
	for _, name := range []string{"missing_dependency", "wrong_identity", "invalid_yaml", "multiple_documents", "ambiguous_kind", "conflicting_schema", "cycle"} {
		t.Run(name, func(t *testing.T) {
			docs := publishedFixtures(t)
			switch name {
			case "missing_dependency":
				delete(docs, commonID)
			case "wrong_identity":
				docs[envID] = docs[commonID]
			case "invalid_yaml":
				docs[envID] = []byte("[broken")
			case "multiple_documents":
				docs[envID] = append(docs[envID], []byte("\n---\nkind: other\n")...)
			case "ambiguous_kind":
				docs[envID] = bytes.Replace(docs[envID], []byte("spec:"), []byte("spec:\n  kinds:\n    - name: api"), 1)
			case "conflicting_schema":
				docs[envID] = bytes.ReplaceAll(docs[envID], []byte("configuration-properties-api-http"), []byte("api-http-interface"))
			case "cycle":
				docs[commonID] = bytes.Replace(docs[commonID], []byte("spec:"), []byte("spec:\n  dependencies:\n    - "+envID), 1)
			}
			if _, err := snapshotWithLoader([]string{envID}, resolver.NewInMemoryLoader(docs), t.TempDir()); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

// Keep the upstream incompatibility executable, rather than deleting version
// annotations or rewriting the generated Profile to hide it. The original
// acceptance suite still requires a successful real published-path handoff.
func TestV060CatalogAPIIdentityIncompatibility(t *testing.T) {
	snapshot := t.TempDir()
	if _, err := snapshotWithLoader([]string{envID}, resolver.NewInMemoryLoader(publishedFixtures(t)), snapshot); err != nil {
		t.Fatal(err)
	}
	_, err := extensioncheck.ResolveExtensionClosure([]extensioncheck.ExtensionReference{{ID: envID}}, extensioncheck.ProfileOptions{CatalogRoots: []string{snapshot}})
	if err == nil || !strings.Contains(err.Error(), "missing extension definition") {
		t.Fatalf("expected released identity mismatch, got %v", err)
	}
}

func TestFailedPublishedValidationNeverReturnsSignedArtifacts(t *testing.T) {
	profile, err := os.ReadFile("../../../artifacts/request-logger-http.profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Validate(profile, "testdata", key, handoff.Digest([]byte("test")))
	if err == nil || a != nil {
		t.Fatalf("failed validation returned artifacts: %v, %v", a, err)
	}
}

func TestPublishedModeRejectsHTTPWithoutLocalFallback(t *testing.T) {
	if _, err := resolveSnapshot([]string{"http://127.0.0.1/extension.yaml"}, "", t.TempDir()); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("%v", err)
	}
}

func TestCatalogRejectsOversizedResponse(t *testing.T) {
	_, err := snapshotWithLoader([]string{envID}, func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(strings.Repeat("x", (4<<20)+1))), nil
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("%v", err)
	}
}
