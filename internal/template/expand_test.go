package template

import "testing"

func TestExtract(t *testing.T) {
	got := Extract("a: ${FOO:-bar}\nb: ${BAZ:-qux}\nc: ${FOO:-other}\n")
	if len(got) != 2 {
		t.Fatalf("want 2 vars, got %d (%v)", len(got), got)
	}
	if got[0].Name != "BAZ" || got[1].Name != "FOO" {
		t.Fatalf("unexpected order: %v", got)
	}
	if got[1].Default != "bar" {
		t.Fatalf("first-occurrence default lost: %q", got[1].Default)
	}
}

func TestRenderUsesEnvThenDefault(t *testing.T) {
	in := "name: ${USER:-anon}\nrole: ${ROLE:-admin}"
	out := Render(in, map[string]string{"USER": "ada"})
	want := "name: ada\nrole: admin"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestRenderLeavesPlainShellAlone(t *testing.T) {
	in := `cmd: ["sh","-c","kill $pid && echo ${UNRELATED}"]`
	if out := Render(in, nil); out != in {
		t.Fatalf("plain $shell mutated: %q", out)
	}
}

func TestParseVarHandlesBracesInDefault(t *testing.T) {
	in := "x: ${VAR:-{nested}}"
	got := Extract(in)
	if len(got) != 1 || got[0].Default != "{nested}" {
		t.Fatalf("nested default not preserved: %#v", got)
	}
	if Render(in, nil) != "x: {nested}" {
		t.Fatalf("nested default render wrong: %q", Render(in, nil))
	}
}

func TestRenderUnknownShapeIsLiteral(t *testing.T) {
	in := "x: ${NOPE}"
	if out := Render(in, map[string]string{"NOPE": "y"}); out != in {
		t.Fatalf("variable without :- should be untouched, got %q", out)
	}
}
