// Package devbind is a deliberately small development-side consumer of a
// Runtime Conditions Profile. It does not validate Runtime Conditions; the
// profiler and extension schemas do that at generation time. It performs only
// the envelope checks it needs to read the Profile defensively, then decides,
// Condition by Condition, whether this local development environment can
// fulfill the demand and which environment variables that fulfillment yields.
package devbind

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	profileAPIVersion = "runtimeconditions.io/v1alpha1"
	profileKind       = "RuntimeConditionsProfile"
)

// localTodosAPI is the contract the local apps/todos-api serves. A Condition
// is bound to it only if everything it declares falls within this contract.
var localTodosAPI = struct {
	format, uri, version string
	operations           map[string]map[string]string
}{
	format:  "openapi",
	uri:     "catalog://api/default/todos-api",
	version: "1.0.0",
	operations: map[string]map[string]string{
		"GET /todos/{id}": {"id": "integer", "title": "string", "completed": "boolean"},
	},
}

// Environment describes the concrete local resources this development
// environment offers.
type Environment struct {
	TodosAPIURL string
	RedisAddr   string
}

// EnvVar is one environment variable supplied to the workload.
type EnvVar struct {
	Name  string
	Value string
}

// Binding records how one portable Condition was fulfilled locally.
type Binding struct {
	Condition string
	Kind      string
	Resource  string
	Env       []EnvVar
}

// InvalidProfileError means the document could not be read as a Profile
// envelope at all.
type InvalidProfileError struct {
	Problems []string
}

func (e *InvalidProfileError) Error() string {
	return "invalid profile:\n  " + strings.Join(e.Problems, "\n  ")
}

// UnsupportedCondition is valid demand that this environment cannot fulfill.
type UnsupportedCondition struct {
	Name   string
	Kind   string
	Reason string
}

// UnsupportedError lists every Condition this environment cannot fulfill.
type UnsupportedError struct {
	Conditions []UnsupportedCondition
}

func (e *UnsupportedError) Error() string {
	lines := make([]string, 0, len(e.Conditions))
	for _, c := range e.Conditions {
		lines = append(lines, fmt.Sprintf("condition %q (kind %q): %s", c.Name, c.Kind, c.Reason))
	}
	return "unsupported conditions in development environment:\n  " + strings.Join(lines, "\n  ")
}

type profileDoc struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Conditions yaml.Node `yaml:"conditions"`
}

type condition struct {
	name          string
	kind          string
	iface         map[string]any
	configuration map[string]any
}

// offer is what a local resource can supply for one Condition.
type offer struct {
	resource string
	provides map[string]string
}

// Bind reads a Profile and binds every Condition to a local resource. It
// returns an *InvalidProfileError or *UnsupportedError when it cannot.
func Bind(data []byte, env Environment) ([]Binding, error) {
	conditions, err := parse(data)
	if err != nil {
		return nil, err
	}

	var bindings []Binding
	var unsupported []UnsupportedCondition
	for _, c := range conditions {
		o, reason := evaluate(c, env)
		if reason == "" {
			var vars []EnvVar
			vars, reason, err = configure(c, o)
			if err != nil {
				return nil, err
			}
			if reason == "" {
				bindings = append(bindings, Binding{Condition: c.name, Kind: c.kind, Resource: o.resource, Env: vars})
				continue
			}
		}
		unsupported = append(unsupported, UnsupportedCondition{Name: c.name, Kind: c.kind, Reason: reason})
	}
	if len(unsupported) > 0 {
		return nil, &UnsupportedError{Conditions: unsupported}
	}
	return bindings, nil
}

func parse(data []byte) ([]condition, error) {
	var doc profileDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, &InvalidProfileError{Problems: []string{err.Error()}}
	}

	var problems []string
	if doc.APIVersion != profileAPIVersion {
		problems = append(problems, fmt.Sprintf("apiVersion is %q, expected %q", doc.APIVersion, profileAPIVersion))
	}
	if doc.Kind != profileKind {
		problems = append(problems, fmt.Sprintf("kind is %q, expected %q", doc.Kind, profileKind))
	}

	var rawConditions []any
	if doc.Conditions.Kind != yaml.SequenceNode {
		problems = append(problems, "conditions must be a list")
	} else if err := doc.Conditions.Decode(&rawConditions); err != nil {
		problems = append(problems, fmt.Sprintf("conditions: %v", err))
	}

	seen := map[string]bool{}
	conditions := make([]condition, 0, len(rawConditions))
	for i, item := range rawConditions {
		raw, _ := item.(map[string]any)
		name, _ := raw["name"].(string)
		if _, present := raw["name"]; !present {
			name = fmt.Sprintf("conditions[%d]", i)
		}
		kind, _ := raw["kind"].(string)
		if name == "" || kind == "" {
			problems = append(problems, fmt.Sprintf("conditions[%d] must have a name and kind", i))
			continue
		}
		if seen[name] {
			problems = append(problems, fmt.Sprintf("conditions[%d]: duplicate condition name %q", i, name))
			continue
		}
		seen[name] = true
		iface, _ := raw["interface"].(map[string]any)
		configuration, _ := raw["configuration"].(map[string]any)
		conditions = append(conditions, condition{name: name, kind: kind, iface: iface, configuration: configuration})
	}

	if len(problems) > 0 {
		return nil, &InvalidProfileError{Problems: problems}
	}
	return conditions, nil
}

// evaluate is the development environment's support table. Every Condition
// either matches an entry here or is reported as unsupported.
func evaluate(c condition, env Environment) (offer, string) {
	switch c.kind {
	case "api":
		if t := str(c.iface, "type"); t != "http" {
			return offer{}, fmt.Sprintf("no local binding for API interface type %q", t)
		}
		if reason := matchLocalTodosAPI(c.iface); reason != "" {
			return offer{}, reason
		}
		return offer{
			resource: "local apps/todos-api at " + env.TodosAPIURL,
			provides: map[string]string{"baseUrl": env.TodosAPIURL},
		}, ""
	case "cache":
		t, engine := str(c.iface, "type"), str(c.iface, "engine")
		if t != "key_value" || engine != "redis" {
			return offer{}, fmt.Sprintf("no local binding for cache type %q engine %q", t, engine)
		}
		host, port, err := net.SplitHostPort(env.RedisAddr)
		if err != nil {
			return offer{}, fmt.Sprintf("local redis address %q is unusable: %v", env.RedisAddr, err)
		}
		return offer{
			resource: "local redis at " + env.RedisAddr,
			provides: map[string]string{
				"url":      "redis://" + env.RedisAddr,
				"hostname": host,
				"port":     port,
			},
		}, ""
	default:
		return offer{}, fmt.Sprintf("no development binding for condition kind %q", c.kind)
	}
}

// matchLocalTodosAPI returns why the declared API demand falls outside the
// local todos-api contract, or "" if the local provider satisfies it.
func matchLocalTodosAPI(iface map[string]any) string {
	operations, _ := iface["operations"].([]any)
	if len(operations) > 0 {
		for _, raw := range operations {
			op, _ := raw.(map[string]any)
			key := strings.ToUpper(str(op, "method")) + " " + str(op, "path")
			served, ok := localTodosAPI.operations[key]
			if !ok {
				return fmt.Sprintf("local todos-api does not serve operation %q", key)
			}
			expected, _ := op["responseSchema"].(map[string]any)
			for field, rawType := range expected {
				fieldType, _ := rawType.(string)
				if served[field] == "" || served[field] != fieldType {
					return fmt.Sprintf("local todos-api %s response field %q is %q, condition expects %v", key, field, served[field], rawType)
				}
			}
		}
		return ""
	}

	spec, _ := iface["spec"].(map[string]any)
	if uri := str(spec, "uri"); uri != localTodosAPI.uri {
		return fmt.Sprintf("no local provider for API spec %q", uri)
	}
	if format := str(spec, "format"); format != localTodosAPI.format {
		return fmt.Sprintf("local todos-api is described by %q, condition declares format %q", localTodosAPI.format, format)
	}
	if raw, declared := spec["version"]; declared {
		constraint, isString := raw.(string)
		if !isString {
			return fmt.Sprintf("version requirement %v is not a string", raw)
		}
		ok, err := satisfiesVersion(constraint, localTodosAPI.version)
		if err != nil {
			return err.Error()
		}
		if !ok {
			return fmt.Sprintf("local todos-api serves version %q, which does not satisfy %q", localTodosAPI.version, constraint)
		}
	}
	return ""
}

// configure maps the properties a local resource provides onto the
// environment variables the Condition asks for. For alternatives it chooses
// the first complete alternative in declared order.
func configure(c condition, o offer) ([]EnvVar, string, error) {
	if len(c.configuration) == 0 {
		return nil, "condition declares no configuration; this environment only supplies env configuration", nil
	}
	for key := range c.configuration {
		if key != "env" && key != "alternatives" {
			return nil, fmt.Sprintf("unsupported configuration form %q", key), nil
		}
	}
	if len(c.configuration) > 1 {
		return nil, "configuration declares both env and alternatives; this environment supports one form per condition", nil
	}

	if raw, ok := c.configuration["env"]; ok {
		entries, err := envEntries(c.name, "configuration.env", raw)
		if err != nil {
			return nil, "", err
		}
		vars, missing := resolveEnv(entries, o.provides)
		if missing != "" {
			return nil, fmt.Sprintf("local binding does not provide property %q", missing), nil
		}
		return vars, "", nil
	}

	alternatives, ok := c.configuration["alternatives"].([]any)
	if !ok {
		return nil, "", &InvalidProfileError{Problems: []string{fmt.Sprintf("condition %q: configuration.alternatives must be a list", c.name)}}
	}
	for i, alt := range alternatives {
		altMap, _ := alt.(map[string]any)
		entries, err := envEntries(c.name, fmt.Sprintf("configuration.alternatives[%d].env", i), altMap["env"])
		if err != nil {
			return nil, "", err
		}
		if vars, missing := resolveEnv(entries, o.provides); missing == "" {
			return vars, "", nil
		}
	}
	return nil, fmt.Sprintf("local binding provides %s but no alternative is fully satisfiable", keys(o.provides)), nil
}

type envEntry struct{ property, name string }

func envEntries(conditionName, path string, raw any) ([]envEntry, error) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, &InvalidProfileError{Problems: []string{fmt.Sprintf("condition %q: %s must be a non-empty list", conditionName, path)}}
	}
	entries := make([]envEntry, 0, len(list))
	for i, item := range list {
		m, _ := item.(map[string]any)
		e := envEntry{property: str(m, "property"), name: str(m, "name")}
		if e.property == "" || e.name == "" {
			return nil, &InvalidProfileError{Problems: []string{fmt.Sprintf("condition %q: %s[%d] must have property and name", conditionName, path, i)}}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func resolveEnv(entries []envEntry, provides map[string]string) ([]EnvVar, string) {
	vars := make([]EnvVar, 0, len(entries))
	for _, e := range entries {
		value, ok := provides[e.property]
		if !ok {
			return nil, e.property
		}
		vars = append(vars, EnvVar{Name: e.name, Value: value})
	}
	return vars, ""
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func keys(m map[string]string) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
