package handoff

import (
	"strings"
	"testing"
)

func profileWithConditions(conditions string) []byte {
	return []byte("apiVersion: runtimeconditions.io/v1alpha1\n" +
		"kind: RuntimeConditionsProfile\n" +
		"extensions:\n  - https://runtimeconditions.io/extensions/rc/common-integrations:v1alpha2\n" +
		"conditions:\n" + conditions)
}

func TestEnvelopeAcceptsConditionsWithoutNames(t *testing.T) {
	ids, err := Envelope(profileWithConditions("  - kind: api\n  - kind: cache\n"))
	if err != nil {
		t.Fatalf("nameless Conditions should be accepted: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected one extension id, got %v", ids)
	}
}

func TestEnvelopeAcceptsUniqueNames(t *testing.T) {
	if _, err := Envelope(profileWithConditions("  - name: a\n    kind: api\n  - name: b\n    kind: cache\n")); err != nil {
		t.Fatalf("unique names should be accepted: %v", err)
	}
}

func TestEnvelopeRejectsBadConditions(t *testing.T) {
	cases := map[string]string{
		"duplicate names":  "  - name: a\n    kind: api\n  - name: a\n    kind: cache\n",
		"empty name":       "  - name: \"\"\n    kind: api\n",
		"missing kind":     "  - name: a\n",
		"nameless no kind": "  - interface: {}\n",
		"non-string name":  "  - name: 3\n    kind: api\n",
	}
	for label, conditions := range cases {
		_, err := Envelope(profileWithConditions(conditions))
		if err == nil || !strings.HasPrefix(err.Error(), "profile_unusable") {
			t.Errorf("%s: expected profile_unusable, got %v", label, err)
		}
	}
}
