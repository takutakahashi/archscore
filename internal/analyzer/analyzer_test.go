package analyzer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestHistoryFromRepositorySubdirectory(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "backend")
	write(t, root, "go.mod", "module example.test/app\n")
	write(t, root, "a/a.go", "package a\n")
	write(t, root, "b/b.go", "package b\n")
	runGit(t, repo, "init")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "both")
	write(t, root, "a/a.go", "package a\n// changed\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "a only")

	cfg := Config{HistoryCommits: 10, Modules: []ModuleConfig{
		{Name: "a", Paths: []string{"a/**"}},
		{Name: "b", Paths: []string{"b/**"}},
	}}
	r, err := Analyze(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Modules[0].ChangePartners) != 1 || r.Modules[0].ChangePartners[0] != "b" {
		t.Fatalf("subdirectory history was not matched: %#v", r.Modules)
	}
	last := r.Components[len(r.Components)-1]
	if last.Name != "change-locality" || last.Detail != "analyzed 2 matching commits (up to 10)" {
		t.Fatalf("unexpected history component: %#v", last)
	}
}

func TestDiscoverGroupsDependentPackagesDeterministically(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.test/app\n")
	write(t, root, "orders/order.go", "package orders\nimport \"example.test/app/payments\"\n")
	write(t, root, "payments/payment.go", "package payments\n")
	write(t, root, "unrelated/audit.go", "package unrelated\n")
	cfg := DiscoverConfig{MinClusters: 2, MaxClusters: 2, HistoryCommits: 10, MinCochange: 2}
	first, err := Discover(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Discover(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Candidates) != 1 || len(second.Candidates) != 1 {
		t.Fatalf("candidates: %#v", first.Candidates)
	}
	if fmt.Sprint(first.Candidates) != fmt.Sprint(second.Candidates) {
		t.Fatal("discovery is not deterministic")
	}
	found := false
	for _, boundary := range first.Candidates[0].Boundaries {
		if strings.Join(boundary.Units, ",") == "orders,payments" {
			found = true
			if boundary.Evidence.InternalDependencyEdges != 1 {
				t.Fatalf("evidence: %#v", boundary.Evidence)
			}
		}
	}
	if !found {
		t.Fatalf("dependent packages were not grouped: %#v", first.Candidates[0])
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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
