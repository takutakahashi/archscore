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

	cfg := Config{HistoryCommits: 10, MinModuleChanges: 1, MinCochange: 1, Modules: []ModuleConfig{
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
	if r.Modules[0].ChangeLocality != .5 || r.Modules[0].ContainedChanges != 1 {
		t.Fatalf("unexpected locality: %#v", r.Modules[0])
	}
	last := r.Components[len(r.Components)-1]
	if last.Name != "change-locality" || last.Detail != "evaluated 2/2 modules; target 80% contained changes" {
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

func TestCompositionRootIsExemptFromFanOutPenalty(t *testing.T) {
	modules := []ModuleConfig{{Name: "root", Role: "composition-root"}}
	files := map[string]*sourceFile{"root/main.go": {Path: "root/main.go", Module: "root"}}
	var edges []edge
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("m%d", i)
		modules = append(modules, ModuleConfig{Name: name})
		files[name+"/x.go"] = &sourceFile{Path: name + "/x.go", Module: name}
		edges = append(edges, edge{FromFile: "root/main.go", ToFile: name + "/x.go", FromModule: "root", ToModule: name})
	}
	report := buildReport(files, edges, Config{Modules: modules, MaxFanOut: 2})
	if report.Components[3].Score != 15 {
		t.Fatalf("composition root was penalized: %#v", report.Components[3])
	}
	modules[0].Role = ""
	report = buildReport(files, edges, Config{Modules: modules, MaxFanOut: 2})
	if report.Components[3].Score >= 15 {
		t.Fatalf("ordinary module was not penalized: %#v", report.Components[3])
	}
}

func TestSinglePackageModuleHasNeutralCohesion(t *testing.T) {
	files := map[string]*sourceFile{
		"a/one.go": {Path: "a/one.go", Module: "a"},
		"a/two.go": {Path: "a/two.go", Module: "a"},
		"b/one.go": {Path: "b/one.go", Module: "b"},
	}
	edges := []edge{{FromFile: "a/one.go", ToFile: "b/one.go", FromModule: "a", ToModule: "b"}}
	reports := moduleReports(files, edges, Config{Modules: []ModuleConfig{{Name: "a"}, {Name: "b"}}})
	if reports[0].Units != 1 || reports[0].Cohesion != 1 {
		t.Fatalf("single Go package should be neutral: %#v", reports[0])
	}
}

func TestDiscoverRespectsBoundaryFileLimit(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.test/app\n")
	write(t, root, "a/one.go", "package a\nimport \"example.test/app/b\"\n")
	write(t, root, "a/two.go", "package a\n")
	write(t, root, "b/one.go", "package b\n")
	write(t, root, "b/two.go", "package b\n")
	report, err := Discover(root, DiscoverConfig{MinClusters: 1, MaxClusters: 2, MaxBoundarySize: 10, MaxBoundaryFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].ClusterCount != 2 {
		t.Fatalf("file limit was ignored: %#v", report.Candidates)
	}
	if len(report.Notes) == 0 {
		t.Fatal("expected an explanation for stopping before min-clusters")
	}
}

func TestPublicContractRejectsPrivateImportsAndCountsExports(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.test/app\n")
	write(t, root, "provider/api/api.go", "package api\ntype Client struct{}\nfunc New() Client { return Client{} }\n")
	write(t, root, "provider/internal/impl.go", "package internal\ntype Secret struct{}\n")
	write(t, root, "consumer/use.go", "package consumer\nimport \"example.test/app/provider/internal\"\n")
	limit := 1
	cfg := Config{Modules: []ModuleConfig{
		{Name: "provider", Paths: []string{"provider/**"}, PublicPaths: []string{"provider/api/**"}, MaxPublicSymbols: &limit},
		{Name: "consumer", Paths: []string{"consumer/**"}, MayDependOn: []string{"provider"}},
	}}
	report, err := Analyze(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Violations) != 1 || report.Violations[0].Kind != "private-dependency" {
		t.Fatalf("private import not rejected: %#v", report.Violations)
	}
	if report.Modules[0].PublicSymbols != 2 {
		t.Fatalf("exports=%d", report.Modules[0].PublicSymbols)
	}
	if report.Modules[0].PrivateInboundViolations != 1 {
		t.Fatalf("private inbound=%d", report.Modules[0].PrivateInboundViolations)
	}
	if report.Components[2].Name != "public-contract" || report.Components[2].Score != 8 {
		t.Fatalf("public contract score: %#v", report.Components[2])
	}
}

func TestCompareReportsDetectsArchitectureRegressions(t *testing.T) {
	baseline := Report{Score: 90, Confidence: 1, Modules: []ModuleReport{{Name: "api", PublicSymbols: 2}}}
	current := Report{Score: 85, Confidence: .8, Violations: []Violation{{Kind: "private-dependency", FromModule: "a", ToModule: "b", FromFile: "a/x.go", Line: 3}}, Cycles: [][]string{{"a", "b"}}, Unknown: []string{"unknown.go"}, Modules: []ModuleReport{{Name: "api", PublicSymbols: 3}}}
	comparison := CompareReports(baseline, current)
	if comparison.Passed || len(comparison.Regressions) != 6 {
		t.Fatalf("unexpected comparison: %#v", comparison)
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
