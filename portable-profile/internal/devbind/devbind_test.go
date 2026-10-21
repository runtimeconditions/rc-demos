package devbind

import (
	"errors"
	"os"
	"strings"
	"testing"
)

var testEnv = Environment{TodosAPIURL: "http://127.0.0.1:18081", RedisAddr: "127.0.0.1:16379"}

func canonicalProfile(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../../artifacts/request-logger-http.profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func profileWith(conditions string) []byte {
	return []byte(`apiVersion: runtimeconditions.io/v1alpha1
kind: RuntimeConditionsProfile
metadata:
  name: test
conditions:
` + conditions)
}

const todosCondition = `
  - name: todos-api
    kind: api
    interface:
      type: http
      spec:
        format: openapi
        uri: catalog://api/default/todos-api
        version: 1.0.0
      operations:
        - method: GET
          path: /todos/{id}
          responseSchema:
            completed: boolean
            id: integer
            title: string
    configuration:
      env:
        - property: baseUrl
          name: TODOS_API_URL
`

const todosSpecOnlyCondition = `
  - name: todos-api
    kind: api
    interface:
      type: http
      spec:
        format: openapi
        uri: catalog://api/default/todos-api
        version: 1.0.0
    configuration:
      env:
        - property: baseUrl
          name: TODOS_API_URL
`

const redisCondition = `
  - name: request-cache
    kind: cache
    interface:
      type: key_value
      engine: redis
    configuration:
      alternatives:
        - env:
            - property: url
              name: REDIS_URL
        - env:
            - property: hostname
              name: REDIS_HOST
            - property: port
              name: REDIS_PORT
`

func envOf(bindings []Binding) map[string]string {
	out := map[string]string{}
	for _, b := range bindings {
		for _, v := range b.Env {
			out[v.Name] = v.Value
		}
	}
	return out
}

func TestCanonicalProfileBindsLocally(t *testing.T) {
	bindings, err := Bind(canonicalProfile(t), testEnv)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}
	got := envOf(bindings)
	want := map[string]string{
		"TODOS_API_URL": "http://127.0.0.1:18081",
		"REDIS_URL":     "redis://127.0.0.1:16379",
	}
	if len(got) != len(want) {
		t.Fatalf("env = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestSupportsTodosAPICondition(t *testing.T) {
	bindings, err := Bind(profileWith(todosCondition), testEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got := envOf(bindings)["TODOS_API_URL"]; got != testEnv.TodosAPIURL {
		t.Fatalf("TODOS_API_URL = %q", got)
	}
}

func TestSupportsRedisConditionWithFirstCompleteAlternative(t *testing.T) {
	bindings, err := Bind(profileWith(redisCondition), testEnv)
	if err != nil {
		t.Fatal(err)
	}
	env := envOf(bindings)
	if env["REDIS_URL"] != "redis://127.0.0.1:16379" {
		t.Fatalf("REDIS_URL = %q", env["REDIS_URL"])
	}
	if _, ok := env["REDIS_HOST"]; ok {
		t.Fatalf("expected exactly one alternative, got %v", env)
	}
}

func TestRejectsTodosAPIDemandOutsideLocalContract(t *testing.T) {
	specCases := map[string]struct{ old, new, reason string }{
		"version":        {"version: 1.0.0", "version: 2.0.0", `does not satisfy "2.0.0"`},
		"constraint":     {"version: 1.0.0", "version: '>1.0.0'", `does not satisfy ">1.0.0"`},
		"bad constraint": {"version: 1.0.0", "version: '1.0'", `"1.0" is not a constraint`},
		"non-string":     {"version: 1.0.0", "version: 1.0", "is not a string"},
		"format":         {"format: openapi", "format: asyncapi", `declares format "asyncapi"`},
		"uri":            {"default/todos-api", "default/other-api", `"catalog://api/default/other-api"`},
	}
	for name, tc := range specCases {
		t.Run(name, func(t *testing.T) {
			_, err := Bind(profileWith(strings.Replace(todosSpecOnlyCondition, tc.old, tc.new, 1)), testEnv)
			var unsupported *UnsupportedError
			if !errors.As(err, &unsupported) || unsupported.Conditions[0].Name != "todos-api" {
				t.Fatalf("expected todos-api to be unsupported, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("error %q does not mention %s", err, tc.reason)
			}
		})
	}

	operationCases := map[string]struct{ old, new, reason string }{
		"operation":      {"path: /todos/{id}", "path: /todos", `does not serve operation "GET /todos"`},
		"method":         {"method: GET", "method: DELETE", `does not serve operation "DELETE /todos/{id}"`},
		"response field": {"completed: boolean", "completed: string", `field "completed"`},
		"unknown field":  {"title: string", "owner: string", `field "owner"`},
	}
	for name, tc := range operationCases {
		t.Run(name, func(t *testing.T) {
			_, err := Bind(profileWith(strings.Replace(todosCondition, tc.old, tc.new, 1)), testEnv)
			var unsupported *UnsupportedError
			if !errors.As(err, &unsupported) || unsupported.Conditions[0].Name != "todos-api" {
				t.Fatalf("expected todos-api to be unsupported, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("error %q does not mention %s", err, tc.reason)
			}
		})
	}
}

func TestOperationsTakePrecedenceOverConflictingSpec(t *testing.T) {
	condition := strings.Replace(todosCondition, "format: openapi", "format: asyncapi", 1)
	condition = strings.Replace(condition, "default/todos-api", "default/other-api", 1)
	condition = strings.Replace(condition, "version: 1.0.0", "version: '>9.0.0'", 1)

	bindings, err := Bind(profileWith(condition), testEnv)
	if err != nil {
		t.Fatalf("explicit operations should take precedence over conflicting spec: %v", err)
	}
	if got := envOf(bindings)["TODOS_API_URL"]; got != testEnv.TodosAPIURL {
		t.Fatalf("TODOS_API_URL = %q", got)
	}
}

func TestBindsTodosAPIWhenVersionConstraintIsSatisfied(t *testing.T) {
	for _, constraint := range []string{"'1.0.0'", "'=1.0.0'", "'^1.0.0'", "'>=1.0.0'", "'~1.0.0'", "'<=1.0.0'", "'<2.0.0'"} {
		t.Run(constraint, func(t *testing.T) {
			if _, err := Bind(profileWith(strings.Replace(todosSpecOnlyCondition, "version: 1.0.0", "version: "+constraint, 1)), testEnv); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSatisfiesVersion(t *testing.T) {
	cases := []struct {
		constraint, version string
		want                bool
	}{
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"=1.0.0", "1.0.0", true},
		{">1.0.0", "1.0.0", false},
		{">1.0.0", "1.0.1", true},
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "0.9.9", false},
		{"<1.0.0", "1.0.0", false},
		{"<=1.0.0", "1.0.0", true},
		{"^1.0.0", "1.0.0", true},
		{"^1.0.0", "1.9.3", true},
		{"^1.0.0", "2.0.0", false},
		{"^1.2.0", "1.1.9", false},
		{"~1.0.0", "1.0.0", true},
		{"~1.0.0", "1.0.9", true},
		{"~1.0.0", "1.1.0", false},
		{"1.10.0", "1.9.0", false},
	}
	for _, tc := range cases {
		got, err := satisfiesVersion(tc.constraint, tc.version)
		if err != nil {
			t.Fatalf("%s vs %s: %v", tc.constraint, tc.version, err)
		}
		if got != tc.want {
			t.Errorf("satisfiesVersion(%q, %q) = %v, want %v", tc.constraint, tc.version, got, tc.want)
		}
	}
	if _, err := satisfiesVersion("1.0.0", "v1"); err == nil {
		t.Error("expected an error for a non-SemVer provider version")
	}
}

func TestRejectsMissingOrNonListConditions(t *testing.T) {
	header := "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsProfile\n"
	for name, tail := range map[string]string{
		"missing": "",
		"null":    "conditions: null\n",
		"mapping": "conditions: {}\n",
		"string":  "conditions: \"\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Bind([]byte(header+tail), testEnv)
			var invalid *InvalidProfileError
			if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "conditions must be a list") {
				t.Fatalf("expected InvalidProfileError for conditions, got %v", err)
			}
		})
	}
	if _, err := Bind([]byte(header+"conditions: []\n"), testEnv); err != nil {
		t.Fatalf("an empty conditions list is a valid envelope: %v", err)
	}
}

func TestRejectsUnknownCondition(t *testing.T) {
	_, err := Bind(profileWith(todosCondition+`
  - name: site-analytics
    kind: google.analytics
    interface:
      type: web
    configuration:
      env:
        - property: measurementId
          name: GA_MEASUREMENT_ID
`), testEnv)
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedError, got %v", err)
	}
	if len(unsupported.Conditions) != 1 || unsupported.Conditions[0].Name != "site-analytics" {
		t.Fatalf("unexpected unsupported conditions: %+v", unsupported.Conditions)
	}
	if !strings.Contains(err.Error(), `"google.analytics"`) {
		t.Fatalf("error does not identify the kind: %v", err)
	}
}

func TestRejectsUnsupportedCacheEngine(t *testing.T) {
	_, err := Bind(profileWith(strings.Replace(redisCondition, "engine: redis", "engine: memcached", 1)), testEnv)
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Conditions[0].Name != "request-cache" {
		t.Fatalf("expected request-cache to be unsupported, got %v", err)
	}
}

func TestRejectsUnprovidedProperty(t *testing.T) {
	_, err := Bind(profileWith(strings.Replace(todosCondition, "property: baseUrl", "property: token", 1)), testEnv)
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) || !strings.Contains(err.Error(), `"token"`) {
		t.Fatalf("expected unsupported property token, got %v", err)
	}
}

func TestRejectsInvalidEnvelope(t *testing.T) {
	_, err := Bind([]byte("apiVersion: v1\nkind: ConfigMap\nconditions:\n  - name: \"\"\n    kind: api\n"), testEnv)
	var invalid *InvalidProfileError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected InvalidProfileError, got %v", err)
	}
	if len(invalid.Problems) != 3 {
		t.Fatalf("expected apiVersion, kind and name problems, got %v", invalid.Problems)
	}
}

func TestReleasedBindingsPermitUnnamedConditions(t *testing.T) {
	data := profileWith(strings.Replace(todosCondition, "  - name: todos-api\n    kind: api", "  - kind: api", 1))
	bindings, err := Bind(data, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Condition != "conditions[0]" {
		t.Fatalf("%+v", bindings)
	}
}
