package rule

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The contract pack is two halves that must agree: the JSON Schemas under
// docs/schemas/ describe the shape, the fixtures under testdata/contracts/ are
// worked examples of it. tracers copies the fixtures and parses them; if a
// fixture stops validating against its schema here, a downstream parser breaks
// there. So this is the gate: every fixture is validated against the schema it
// claims to implement, in looptap's own CI, before tracers ever sees it.
// Golden drift = contract break.

const (
	contractsDir = "../../testdata/contracts"
	schemasDir   = "../../docs/schemas"

	ruleSchemaID        = "https://looptap.dev/schemas/tracers.rule.v1.json"
	analyzeReqSchemaID  = "https://looptap.dev/schemas/tracers.analyze.v1.request.json"
	analyzeRespSchemaID = "https://looptap.dev/schemas/tracers.analyze.v1.response.json"
)

// loadJSON decodes a JSON file into the loose any-tree both the validator and
// the structural comparison want.
func loadJSON(t *testing.T, path string) any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return doc
}

// newContractCompiler registers every schema in docs/schemas/ under its own $id
// so cross-file $refs (the analyze response points at the rule card) resolve
// without touching the network.
func newContractCompiler(t *testing.T) *jsonschema.Compiler {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(schemasDir, "*.json"))
	if err != nil {
		t.Fatalf("glob schemas: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no schemas found in %s", schemasDir)
	}

	c := jsonschema.NewCompiler()
	c.AssertFormat() // make "format": "date-time" load-bearing, not decorative
	for _, path := range files {
		doc := loadJSON(t, path)
		id, ok := doc.(map[string]any)["$id"].(string)
		if !ok || id == "" {
			t.Fatalf("schema %s missing $id", path)
		}
		if err := c.AddResource(id, doc); err != nil {
			t.Fatalf("add resource %s: %v", id, err)
		}
	}
	return c
}

// TestContractFixturesValidate is the heart of the contract pack: each fixture
// is checked against the exact schema (or sub-schema) it claims to implement.
func TestContractFixturesValidate(t *testing.T) {
	c := newContractCompiler(t)

	cases := []struct {
		fixture string
		schema  string // schema $id, optionally with a JSON-pointer fragment
	}{
		{"tracers.rule.v1.golden-bundle.json", ruleSchemaID},
		{"tracers.rule.v1.empty-bundle.json", ruleSchemaID},
		// A leak is still a structurally valid bundle — redaction is a separate
		// gate (redact_test.go), not a schema concern.
		{"tracers.rule.v1.leaky-bundle.json", ruleSchemaID},
		// The card fixture is a single card, so validate it against the card
		// sub-schema rather than the bundle envelope.
		{"tracers.rule.v1.golden-card.json", ruleSchemaID + "#/$defs/card"},
		{"tracers.analyze.v1.request.golden.json", analyzeReqSchemaID},
		{"tracers.analyze.v1.response.golden.json", analyzeRespSchemaID},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			sch, err := c.Compile(tc.schema)
			if err != nil {
				t.Fatalf("compile %s: %v", tc.schema, err)
			}
			inst := loadJSON(t, filepath.Join(contractsDir, tc.fixture))
			if err := sch.Validate(inst); err != nil {
				t.Errorf("%s does not satisfy %s:\n%v", tc.fixture, tc.schema, err)
			}
		})
	}
}

// TestGoldenBundleRoundTrip pins the rule package's own output to the golden
// bundle fixture. Decode the golden card, re-wrap it through the exact path the
// patterns command uses (MarshalBundle), and the re-emitted shape must match
// the fixture key-for-key. generated_at is allowed to differ — it's a clock
// reading, not part of the contract's shape.
func TestGoldenBundleRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractsDir, "tracers.rule.v1.golden-bundle.json"))
	if err != nil {
		t.Fatalf("read golden bundle: %v", err)
	}

	var in Bundle
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("decode golden bundle into Bundle: %v", err)
	}
	if len(in.Cards) != 1 {
		t.Fatalf("golden bundle has %d cards, want 1", len(in.Cards))
	}

	if in.GateMinSessions != 5 {
		t.Errorf("golden gate_min_sessions = %d, want 5", in.GateMinSessions)
	}

	out, err := MarshalBundle(in.Cards, in.GateMinSessions)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var golden, got map[string]any
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}

	if !sameShape(golden, got) {
		t.Errorf("rebuilt bundle drifted from golden shape:\n golden keys: %v\n   our keys: %v",
			keysOf(golden), keysOf(got))
	}
	// cards must never serialize as null — "nothing crossed the gate" is [].
	if _, ok := got["cards"].([]any); !ok {
		t.Errorf("cards is %T, want a JSON array", got["cards"])
	}
}

// sameShape reports whether two decoded-JSON values share a structure: objects
// with the same key sets (recursively), arrays whose elements share a shape.
// Scalar values are ignored — this is a contract-shape check, not an equality
// check, so timestamps and wording are free to differ.
func sameShape(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, va := range av {
			vb, ok := bv[k]
			if !ok || !sameShape(va, vb) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return false
		}
		// Empty arrays match any array; otherwise every element must share the
		// shape of the first element on the other side. Lengths may differ —
		// one evidence turn vs three is still the same contract.
		if len(av) == 0 || len(bv) == 0 {
			return true
		}
		for _, e := range av {
			if !sameShape(e, bv[0]) {
				return false
			}
		}
		for _, e := range bv {
			if !sameShape(e, av[0]) {
				return false
			}
		}
		return true
	default:
		// Scalars (incl. nil) share a shape with any other scalar, never with a
		// container.
		switch b.(type) {
		case map[string]any, []any:
			return false
		}
		return true
	}
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
