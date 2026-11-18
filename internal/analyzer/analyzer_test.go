package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeGoAndTypeScript(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.test/app\n")
	write(t, root, "domain/order.go", "package domain\nimport \"example.test/app/infra\"\n")
	write(t, root, "infra/db.go", "package infra\nimport \"example.test/app/domain\"\n")
	write(t, root, "ui/view.ts", "import { run } from '../app/run';\nexport { db } from '../infra/db';\n")
	write(t, root, "app/run.ts", "import '../domain/model';\nexport const run = 1;\n")
	write(t, root, "domain/model.ts", "export const model = 1;\n")
	write(t, root, "infra/db.ts", "export const db = 1;\n")
	cfg := Config{MaxFanOut: 3, Modules: []ModuleConfig{
		{Name: "domain", Paths: []string{"domain/**"}},
		{Name: "app", Paths: []string{"app/**"}, MayDependOn: []string{"domain"}},
		{Name: "infra", Paths: []string{"infra/**"}, MayDependOn: []string{"domain"}},
		{Name: "ui", Paths: []string{"ui/**"}, MayDependOn: []string{"app"}},
	}}
	r, err := Analyze(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.Files != 6 {
		t.Fatalf("files=%d", r.Files)
	}
	if len(r.Violations) != 2 {
		t.Fatalf("violations=%d: %#v", len(r.Violations), r.Violations)
	}
	if len(r.Cycles) != 1 {
		t.Fatalf("cycles=%d: %#v", len(r.Cycles), r.Cycles)
	}
	if r.Gate != "fail" {
		t.Fatalf("gate=%s", r.Gate)
	}
}

func TestTypeScriptCommentsAreIgnored(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a/a.ts", "// import '../b/b'\n/* require('../b/b') */\nexport const a=1")
	write(t, root, "b/b.ts", "export const b=1")
	cfg := Config{Modules: []ModuleConfig{{Name: "a", Paths: []string{"a/**"}}, {Name: "b", Paths: []string{"b/**"}}}}
	r, err := Analyze(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.Edges != 0 || len(r.Violations) != 0 {
		t.Fatalf("unexpected imports: %#v", r)
	}
}

func TestGlob(t *testing.T) {
	for _, tc := range []struct {
		pattern, value string
		want           bool
	}{
		{"src/**", "src/a/b.ts", true},
		{"src/*.ts", "src/a.ts", true},
		{"src/*.ts", "src/a/b.ts", false},
	} {
		if got := glob(tc.pattern, tc.value); got != tc.want {
			t.Errorf("glob(%q,%q)=%v", tc.pattern, tc.value, got)
		}
	}
}

func write(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
