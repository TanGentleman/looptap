package signal

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/TanGentleman/looptap/internal/parser"
)

func turns(specs ...parser.Turn) parser.Session {
	for i := range specs {
		specs[i].Idx = i
	}
	return parser.Session{ID: "s1", Turns: specs}
}

func user(text string) parser.Turn      { return parser.Turn{Role: "user", Content: text} }
func assistant(text string) parser.Turn { return parser.Turn{Role: "assistant", Content: text} }
func toolUse(name, args string) parser.Turn {
	return parser.Turn{Role: "tool_use", ToolName: name, Content: args}
}
func toolResult(name, out string, isErr bool) parser.Turn {
	return parser.Turn{Role: "tool_result", ToolName: name, Content: out, IsError: isErr}
}

// summarize renders signals as "type@turn conf evidence" so cases read as one line each.
func summarize(sigs []Signal) []string {
	var out []string
	for _, s := range sigs {
		idx := -1
		if s.TurnIdx != nil {
			idx = *s.TurnIdx
		}
		out = append(out, fmt.Sprintf("%s@%d %.2f %s", s.Type, idx, s.Confidence, s.Evidence))
	}
	return out
}

func TestDetectors(t *testing.T) {
	bash := `{"command":"go test ./internal/db"}`
	cases := []struct {
		name    string
		det     Detector
		session parser.Session
		want    []string
	}{
		{"misalignment: correction phrase", &Misalignment{},
			turns(user("add a flag"), assistant("done"), user("that's wrong, use the config file")),
			[]string{"misalignment@2 0.80 that's wrong"}},
		{"misalignment: rephrased request", &Misalignment{},
			turns(user("add a login page with email"), assistant("which framework?"), user("add a login page with email please")),
			[]string{"misalignment@2 0.86 user rephrased previous turn"}},
		{"misalignment: distinct requests", &Misalignment{},
			turns(user("add a login page"), assistant("done"), user("now write the migration")),
			nil},
		{"misalignment: assistant text ignored", &Misalignment{},
			turns(user("refactor db.go"), assistant("you misunderstood the API, that's wrong")),
			nil},

		{"stagnation: assistant repeats itself", &Stagnation{},
			turns(assistant("the test fails because the schema is missing"), user("and?"), assistant("the test fails because the schema is missing")),
			[]string{"stagnation@2 1.00 assistant repeating itself"}},
		{"stagnation: different answers", &Stagnation{},
			turns(assistant("the test fails because the schema is missing"), assistant("added the migration and the test passes")),
			nil},

		{"disengagement: phrase and short last turn", &Disengagement{},
			turns(user("fix the parser"), assistant("trying"), user("never mind")),
			[]string{"disengagement@2 0.80 never mind", "disengagement@2 0.50 short final turn (not satisfaction)"}},
		{"disengagement: short thanks is not giving up", &Disengagement{},
			turns(user("fix the parser"), assistant("fixed"), user("thanks")),
			nil},
		{"disengagement: long last turn", &Disengagement{},
			turns(user("fix the parser"), assistant("fixed"), user("now run the full suite and push the branch to origin")),
			nil},
		{"disengagement: no user turns", &Disengagement{},
			turns(assistant("hello")),
			nil},

		{"satisfaction: thanks at the end", &Satisfaction{},
			turns(user("fix the parser"), assistant("fixed"), user("perfect, ship it")),
			[]string{"satisfaction@2 0.80 perfect"}},
		{"satisfaction: only the last three user turns count", &Satisfaction{},
			turns(user("thanks for the plan"), user("step one"), user("step two"), user("step three")),
			nil},

		{"failure: is_error tool result", &Failure{},
			turns(toolUse("Bash", bash), toolResult("Bash", "ok", true)),
			[]string{"failure@1 0.90 tool_result with is_error=true"}},
		{"failure: pattern in output and assistant text", &Failure{},
			turns(toolResult("Bash", "Traceback (most recent call last):", false), assistant("The build failed again.")),
			[]string{"failure@0 0.60 pattern: traceback", "failure@1 0.60 pattern: build failed"}},
		{"failure: user text ignored", &Failure{},
			turns(user("why does it say exit code 1?")),
			nil},
		{"failure: clean output", &Failure{},
			turns(toolResult("Bash", "PASS\nok  github.com/x 0.01s", false)),
			nil},

		{"loop: same call three times", &Loop{},
			turns(toolUse("Bash", bash), toolResult("Bash", "FAIL", false), toolUse("Bash", bash), toolResult("Bash", "FAIL", false), toolUse("Bash", bash)),
			[]string{"loop@4 1.00 Bash called 3 times with similar args"}},
		{"loop: two repeats are not a loop", &Loop{},
			turns(toolUse("Bash", bash), toolUse("Bash", bash)),
			nil},
		{"loop: different tools", &Loop{},
			turns(toolUse("Bash", bash), toolUse("Read", bash), toolUse("Edit", bash)),
			nil},
		{"loop: same tool, different args", &Loop{},
			turns(toolUse("Read", `{"file":"a.go"}`), toolUse("Read", `{"file":"b.go"}`), toolUse("Read", `{"file":"c.go"}`)),
			nil},

		{"exhaustion: rate limit in tool output", &Exhaustion{},
			turns(toolResult("WebFetch", "HTTP 429 Too Many Requests", false)),
			[]string{"exhaustion@0 0.70 too many requests"}},
		{"exhaustion: user text ignored", &Exhaustion{},
			turns(user("watch out for the rate limit")),
			nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarize(tc.det.Detect(tc.session))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s.Detect:\n got  %q\n want %q", tc.det.Name(), got, tc.want)
			}
			for _, s := range tc.det.Detect(tc.session) {
				if s.SessionID != "s1" || s.Type != tc.det.Name() || s.Category != tc.det.Category() {
					t.Errorf("signal %+v does not match detector %s/%s", s, tc.det.Name(), tc.det.Category())
				}
			}
		})
	}
}

func TestAllDetectorsRegistered(t *testing.T) {
	want := map[string]string{
		"misalignment":  "interaction",
		"stagnation":    "interaction",
		"disengagement": "interaction",
		"satisfaction":  "interaction",
		"failure":       "execution",
		"loop":          "execution",
		"exhaustion":    "environment",
	}
	got := map[string]string{}
	for _, d := range All {
		got[d.Name()] = d.Category()
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("registered detectors = %v, want %v", got, want)
	}
}

func TestPhraseFilesLoad(t *testing.T) {
	for name, list := range map[string][]string{
		"misalignment":  misalignmentPhrases,
		"disengagement": disengagementPhrases,
		"satisfaction":  satisfactionPhrases,
		"exhaustion":    exhaustionPhrases,
	} {
		if len(list) == 0 {
			t.Errorf("%s phrase list is empty", name)
		}
	}
}

func TestRunAllEmptySession(t *testing.T) {
	if got := RunAll(parser.Session{ID: "empty"}); len(got) != 0 {
		t.Errorf("RunAll(empty) = %v, want none", summarize(got))
	}
}
