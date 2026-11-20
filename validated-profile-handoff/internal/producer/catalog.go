package producer

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/runtimeconditions/rc-demos/validated-profile-handoff/handoff"
	resolver "github.com/runtimeconditions/rc-extension-resolver"
	"gopkg.in/yaml.v3"
)

// Catalog resolution belongs exclusively to the trusted producer. Empty catalog
// selects HTTPS. An explicit directory selects offline mode, never fallback.
func resolveSnapshot(ids []string, catalog, snapshot string) (map[string][]byte, error) {
	var loader resolver.LoaderFunc
	if catalog == "" {
		client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("catalog redirect must use HTTPS")
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many catalog redirects")
			}
			return nil
		}}
		loader = func(id string) (io.ReadCloser, error) {
			u, err := url.Parse(id)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
				return nil, fmt.Errorf("expected a complete HTTPS definition URI: %s", id)
			}
			resp, err := client.Get(id)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, fmt.Errorf("catalog %s: HTTP %d", id, resp.StatusCode)
			}
			return resp.Body, nil
		}
	} else {
		docs := map[string][]byte{}
		err := filepath.WalkDir(catalog, func(path string, entry fs.DirEntry, err error) error {
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
			var doc resolver.ExtensionDefinition
			if err := yaml.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if doc.Kind != "RuntimeConditionsExtensionDefinition" {
				return nil
			}
			if doc.Metadata.ID == "" {
				return fmt.Errorf("%s: missing extension ID", path)
			}
			if _, exists := docs[doc.Metadata.ID]; exists {
				return fmt.Errorf("duplicate extension ID %s", doc.Metadata.ID)
			}
			docs[doc.Metadata.ID] = data
			return nil
		})
		if err != nil {
			return nil, err
		}
		loader = resolver.NewInMemoryLoader(docs)
	}
	return snapshotWithLoader(ids, loader, snapshot)
}

// The upstream resolver owns dependency traversal and cycle/identity checks.
// Capture once per URI so multiple roots cannot observe different remote bytes.
func snapshotWithLoader(ids []string, source resolver.LoaderFunc, snapshot string) (map[string][]byte, error) {
	definitions := map[string][]byte{}
	loader := func(id string) (io.ReadCloser, error) {
		if data, ok := definitions[id]; ok {
			return io.NopCloser(bytes.NewReader(data)), nil
		}
		body, err := source(id)
		if err != nil {
			return nil, err
		}
		defer body.Close()
		const limit = 4 << 20
		data, err := io.ReadAll(io.LimitReader(body, limit+1))
		if err != nil {
			return nil, err
		}
		if len(data) > limit {
			return nil, fmt.Errorf("extension too large: %s", id)
		}
		var doc resolver.ExtensionDefinition
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&doc); err != nil {
			return nil, err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return nil, fmt.Errorf("expected one extension document: %s", id)
		}
		if doc.APIVersion != "runtimeconditions.io/v1alpha1" || doc.Kind != "RuntimeConditionsExtensionDefinition" || doc.Metadata.ID != id {
			return nil, fmt.Errorf("invalid extension envelope or identity: %s", id)
		}
		definitions[id] = data
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	graph := &resolver.ResolvedGraph{ByID: map[string]*resolver.ExtensionDefinition{}}
	for _, id := range ids {
		resolved, err := resolver.NewResolver(loader).Resolve(id)
		if err != nil {
			return nil, err
		}
		for key, ext := range resolved.ByID {
			graph.ByID[key] = ext
		}
	}
	// Stable ordering makes conflict reporting and snapshot publication inspectable.
	keys := make([]string, 0, len(graph.ByID))
	for id := range graph.ByID {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	kinds := map[string]string{}
	for _, id := range keys {
		ext := graph.ByID[id]
		for _, kind := range ext.Spec.Kinds {
			if owner, exists := kinds[kind.Name]; exists {
				return nil, fmt.Errorf("ambiguous kind %s: %s and %s", kind.Name, owner, id)
			}
			kinds[kind.Name] = id
		}
		graph.Extensions = append(graph.Extensions, ext)
	}
	// The generic resolver allows kind fallback; the experiment deliberately rejects
	// ambiguity and retains the profiler's stricter definition/Profile validation.
	if _, err := resolver.Merge(graph); err != nil {
		return nil, err
	}
	for _, id := range keys {
		data := definitions[id]
		if err := os.WriteFile(filepath.Join(snapshot, handoff.Digest(data)+".yaml"), data, 0600); err != nil {
			return nil, err
		}
	}
	return definitions, nil
}
