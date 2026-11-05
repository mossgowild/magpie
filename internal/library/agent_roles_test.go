package library

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Decode the public format, including the role an older reader ignores.
func agentWithMode(t *testing.T, s Subagent, mode string) Subagent {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	raw["mode"] = mode
	b, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLibraryAgentRoleChangesClientsFiles(t *testing.T) {
	h := sandbox(t)
	files := subagentAgents(t, h)
	s := reviewer
	for id := range files {
		s.Agents = append(s.Agents, id)
	}
	// A pre-role record keeps the original subagent output.
	ok(t)(SaveSubagent("", s))
	if got := read(t, files["opencode"]); !strings.Contains(got, "\nmode: subagent\n") {
		t.Fatalf("legacy agent is not a subagent:\n%s", got)
	}
	primary := agentWithMode(t, s, "primary")
	r, err := SaveSubagent(s.Name, primary)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, files["opencode"]); !strings.Contains(got, "\nmode: primary\n") || !strings.Contains(got, "Report bugs only.") {
		t.Errorf("OpenCode did not get a primary agent with its prompt:\n%s", got)
	}
	if len(r.Problems) != len(files)-1 {
		t.Errorf("unsupported primary writers must report a problem: %+v", r.Problems)
	}
	for id, f := range files {
		if id == "opencode" {
			continue
		}
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s kept a subagent after its role became primary: %v", id, err)
		}
	}
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Subagents) != 1 || len(v.Subagents[0].Files) != 1 || len(v.Subagents[0].Problems) != len(files)-1 {
		t.Errorf("the role's written files and problems: %+v", v.Subagents)
	}
	var raw struct {
		Agents []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"subagentAgents"`
	}
	if err := json.Unmarshal([]byte(mustJSON(t, v)), &raw); err != nil {
		t.Fatal(err)
	}
	for _, a := range raw.Agents {
		if a.Primary != (a.ID == "opencode") {
			t.Errorf("primary capability for %s: %v", a.ID, a.Primary)
		}
	}
	// Switching back rewrites OpenCode and gives every other client its
	// subagent again, without another name or folder.
	ok(t)(SaveSubagent(s.Name, agentWithMode(t, s, "subagent")))
	for id, f := range files {
		if got := read(t, f); got == "" || (id == "opencode" && !strings.Contains(got, "\nmode: subagent\n")) {
			t.Errorf("%s did not get its subagent back:\n%s", id, got)
		}
	}
}

func TestLibraryAgentRoleKeepsUsersFile(t *testing.T) {
	h := sandbox(t)
	files := subagentAgents(t, h)
	s := reviewer
	s.Agents = []string{"opencode", "claude"}
	ok(t)(SaveSubagent("", s))
	owned := map[string]string{}
	for _, id := range s.Agents {
		owned[id] = read(t, files[id]) + "\nThe user's instructions.\n"
		write(t, files[id], owned[id])
	}
	r, err := SaveSubagent(s.Name, agentWithMode(t, s, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Problems) != 2 {
		t.Errorf("role change must report the unsupported client and edited file: %+v", r.Problems)
	}
	for id, want := range owned {
		if got := read(t, files[id]); got != want {
			t.Errorf("role change overwrote or removed %s's edited file:\n%s", id, got)
		}
	}
}

func TestLibraryAgentRoleSurvivesCarry(t *testing.T) {
	sandbox(t)
	s := reviewer
	s.Agents = []string{"opencode"}
	ok(t)(SaveSubagent("", agentWithMode(t, s, "primary")))
	b, err := Collect()
	if err != nil {
		t.Fatal(err)
	}
	carried := mustJSON(t, b)
	if !strings.Contains(carried, `"mode":"primary"`) {
		t.Fatalf("the primary role was not carried: %s", carried)
	}
	h2 := sandbox(t)
	files := subagentAgents(t, h2)
	if r := putJSON(t, carried); len(r.Problems) != 0 {
		t.Fatalf("restore: %+v", r.Problems)
	}
	if got := read(t, files["opencode"]); !strings.Contains(got, "\nmode: primary\n") {
		t.Errorf("restoring changed the agent's role:\n%s", got)
	}
}

func TestLibraryAgentRoleRejectsUnknownMode(t *testing.T) {
	for _, mode := range []string{"all", "PRIMARY", "primary\nmodel: injected"} {
		t.Run(mode, func(t *testing.T) {
			sandbox(t)
			if _, err := SaveSubagent("", agentWithMode(t, reviewer, mode)); err == nil {
				t.Errorf("unsupported mode %q was saved", mode)
			}
		})
	}
}
