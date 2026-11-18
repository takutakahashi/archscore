package analyzer

import (
	"bufio"
	"fmt"
	"go/parser"
	"go/token"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	tsImport = regexp.MustCompile(`(?m)(?:import|export)\s+(?:[^;\n]*?\s+from\s+)?["']([^"']+)["']|require\s*\(\s*["']([^"']+)["']\s*\)|import\s*\(\s*["']([^"']+)["']\s*\)`)
	goModule = regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`)
)

func Analyze(root string, cfg Config) (Report, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	files, modulePath, err := collect(abs, cfg)
	if err != nil {
		return Report{}, err
	}
	edges := resolveEdges(files, modulePath)
	report := buildReport(files, edges, cfg)
	applyHistory(abs, files, cfg, &report)
	return report, nil
}

func collect(root string, cfg Config) (map[string]*sourceFile, string, error) {
	files := map[string]*sourceFile{}
	modulePath := ""
	if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		if m := goModule.FindSubmatch(b); len(m) == 2 {
			modulePath = string(m[1])
		}
	}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if excluded(rel, cfg.Exclude) {
			return nil
		}
		ext := filepath.Ext(rel)
		if ext != ".go" && ext != ".ts" && ext != ".tsx" && ext != ".mts" && ext != ".cts" {
			return nil
		}
		mod := assignModule(rel, cfg.Modules)
		imps, err := parseImports(p, ext)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		files[rel] = &sourceFile{Path: rel, Module: mod, Imports: imps}
		return nil
	})
	return files, modulePath, err
}

func parseImports(path, ext string) ([]sourceImport, error) {
	if ext == ".go" {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		out := make([]sourceImport, 0, len(f.Imports))
		for _, imp := range f.Imports {
			v, _ := strconv.Unquote(imp.Path.Value)
			out = append(out, sourceImport{Path: v, Line: fset.Position(imp.Pos()).Line})
		}
		return out, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	clean := stripComments(string(b))
	matches := tsImport.FindAllStringSubmatchIndex(clean, -1)
	out := make([]sourceImport, 0, len(matches))
	for _, m := range matches {
		value, pos := "", -1
		for g := 1; g <= 3; g++ {
			if m[g*2] >= 0 {
				pos = m[g*2]
				value = clean[m[g*2]:m[g*2+1]]
				break
			}
		}
		if pos >= 0 {
			out = append(out, sourceImport{Path: value, Line: 1 + strings.Count(clean[:pos], "\n")})
		}
	}
	return out, nil
}

func stripComments(s string) string {
	b := []byte(s)
	line, block, quote := false, false, byte(0)
	for i := 0; i < len(b); i++ {
		if line {
			if b[i] == '\n' {
				line = false
			} else {
				b[i] = ' '
			}
			continue
		}
		if block {
			if i+1 < len(b) && b[i] == '*' && b[i+1] == '/' {
				b[i], b[i+1] = ' ', ' '
				i++
				block = false
			} else if b[i] != '\n' {
				b[i] = ' '
			}
			continue
		}
		if quote != 0 {
			if b[i] == '\\' {
				i++
			} else if b[i] == quote {
				quote = 0
			}
			continue
		}
		if b[i] == '\'' || b[i] == '"' || b[i] == '`' {
			quote = b[i]
			continue
		}
		if i+1 < len(b) && b[i] == '/' && b[i+1] == '/' {
			b[i], b[i+1] = ' ', ' '
			i++
			line = true
		}
		if i+1 < len(b) && b[i] == '/' && b[i+1] == '*' {
			b[i], b[i+1] = ' ', ' '
			i++
			block = true
		}
	}
	return string(b)
}

func resolveEdges(files map[string]*sourceFile, goMod string) []edge {
	var out []edge
	for _, f := range files {
		for _, imp := range f.Imports {
			target := ""
			if strings.HasPrefix(filepath.Ext(f.Path), ".go") && goMod != "" && (imp.Path == goMod || strings.HasPrefix(imp.Path, goMod+"/")) {
				dir := strings.TrimPrefix(strings.TrimPrefix(imp.Path, goMod), "/")
				target = firstGoFile(files, dir)
			} else if strings.HasPrefix(imp.Path, ".") {
				base := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(f.Path), imp.Path)))
				target = resolveTS(files, base)
			}
			if target != "" {
				t := files[target]
				out = append(out, edge{f.Path, target, f.Module, t.Module, imp.Path, imp.Line})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FromFile == out[j].FromFile {
			return out[i].Line < out[j].Line
		}
		return out[i].FromFile < out[j].FromFile
	})
	return out
}

func firstGoFile(files map[string]*sourceFile, dir string) string {
	var found []string
	for p := range files {
		if filepath.Ext(p) == ".go" && filepath.ToSlash(filepath.Dir(p)) == dir {
			found = append(found, p)
		}
	}
	sort.Strings(found)
	if len(found) > 0 {
		return found[0]
	}
	return ""
}

func resolveTS(files map[string]*sourceFile, base string) string {
	for _, p := range []string{base, base + ".ts", base + ".tsx", base + ".mts", base + ".cts", base + "/index.ts", base + "/index.tsx"} {
		if _, ok := files[p]; ok {
			return p
		}
	}
	return ""
}

func assignModule(path string, modules []ModuleConfig) string {
	for _, m := range modules {
		for _, pattern := range m.Paths {
			if glob(pattern, path) {
				return m.Name
			}
		}
	}
	return ""
}

func excluded(path string, patterns []string) bool {
	for _, p := range patterns {
		if glob(p, path) {
			return true
		}
	}
	return false
}

func glob(pattern, value string) bool {
	q := regexp.QuoteMeta(filepath.ToSlash(pattern))
	q = strings.ReplaceAll(q, `\*\*`, `.*`)
	q = strings.ReplaceAll(q, `\*`, `[^/]*`)
	q = strings.ReplaceAll(q, `\?`, `[^/]`)
	ok, _ := regexp.MatchString("^"+q+"$", filepath.ToSlash(value))
	return ok
}

func buildReport(files map[string]*sourceFile, edges []edge, cfg Config) Report {
	r := Report{Version: "1", Files: len(files), Edges: len(edges), Gate: "pass"}
	allowed := map[string]map[string]bool{}
	for _, m := range cfg.Modules {
		allowed[m.Name] = map[string]bool{m.Name: true}
		for _, d := range m.MayDependOn {
			allowed[m.Name][d] = true
		}
	}
	for _, e := range edges {
		if e.FromModule != "" && e.ToModule != "" && !allowed[e.FromModule][e.ToModule] {
			r.Violations = append(r.Violations, Violation{"forbidden-dependency", e.FromModule, e.ToModule, e.FromFile, e.ToFile, e.Line, e.Import})
		}
	}
	r.Cycles = moduleCycles(edges)
	if len(r.Violations) > 0 || len(r.Cycles) > 0 {
		r.Gate = "fail"
	}
	r.Modules = moduleReports(files, edges, cfg)
	for _, f := range files {
		if f.Module == "" {
			r.Unknown = append(r.Unknown, f.Path)
		}
	}
	sort.Strings(r.Unknown)
	boundary := maxInt(0, 35-len(r.Violations)*7)
	cycles := maxInt(0, 25-len(r.Cycles)*10)
	over := 0
	for _, m := range r.Modules {
		if m.FanOut > cfg.MaxFanOut {
			over += m.FanOut - cfg.MaxFanOut
		}
	}
	coupling := maxInt(0, 15-over*3)
	cohesionAvg := 1.0
	if len(r.Modules) > 0 {
		cohesionAvg = 0
		for _, m := range r.Modules {
			cohesionAvg += m.Cohesion
		}
		cohesionAvg /= float64(len(r.Modules))
	}
	cohesion := int(math.Round(10 * cohesionAvg))
	coverage := 10
	if len(files) > 0 {
		coverage = int(math.Round(10 * float64(len(files)-len(r.Unknown)) / float64(len(files))))
	}
	r.Components = []ComponentScore{
		{"boundaries", boundary, 35, fmt.Sprintf("%d forbidden dependency edges", len(r.Violations))},
		{"acyclicity", cycles, 25, fmt.Sprintf("%d module cycles", len(r.Cycles))},
		{"coupling", coupling, 15, fmt.Sprintf("fan-out threshold %d", cfg.MaxFanOut)},
		{"cohesion", cohesion, 10, "ratio of internal to all resolved edges"},
		{"module-coverage", coverage, 10, fmt.Sprintf("%d unassigned files", len(r.Unknown))},
	}
	for _, c := range r.Components {
		r.Score += c.Score
	}
	return r
}

func moduleReports(files map[string]*sourceFile, edges []edge, cfg Config) []ModuleReport {
	reports := map[string]*ModuleReport{}
	for _, c := range cfg.Modules {
		reports[c.Name] = &ModuleReport{Name: c.Name}
	}
	for _, f := range files {
		if reports[f.Module] != nil {
			reports[f.Module].Files++
		}
	}
	in, out := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, e := range edges {
		if reports[e.FromModule] == nil || reports[e.ToModule] == nil {
			continue
		}
		if e.FromModule == e.ToModule {
			reports[e.FromModule].InternalEdges++
		} else {
			reports[e.FromModule].ExternalEdges++
			if out[e.FromModule] == nil {
				out[e.FromModule] = map[string]bool{}
			}
			out[e.FromModule][e.ToModule] = true
			if in[e.ToModule] == nil {
				in[e.ToModule] = map[string]bool{}
			}
			in[e.ToModule][e.FromModule] = true
		}
	}
	result := make([]ModuleReport, 0, len(cfg.Modules))
	for _, c := range cfg.Modules {
		r := reports[c.Name]
		r.FanIn, r.FanOut = len(in[c.Name]), len(out[c.Name])
		if r.FanIn+r.FanOut > 0 {
			r.Instability = round(float64(r.FanOut) / float64(r.FanIn+r.FanOut))
		}
		total := r.InternalEdges + r.ExternalEdges
		if total == 0 {
			r.Cohesion = 1
		} else {
			r.Cohesion = round(float64(r.InternalEdges) / float64(total))
		}
		result = append(result, *r)
	}
	return result
}

func moduleCycles(edges []edge) [][]string {
	graph := map[string]map[string]bool{}
	for _, e := range edges {
		if e.FromModule != "" && e.ToModule != "" && e.FromModule != e.ToModule {
			if graph[e.FromModule] == nil {
				graph[e.FromModule] = map[string]bool{}
			}
			graph[e.FromModule][e.ToModule] = true
		}
	}
	index, indices, low, stack, on := 0, map[string]int{}, map[string]int{}, []string{}, map[string]bool{}
	var cycles [][]string
	var visit func(string)
	visit = func(v string) {
		index++
		indices[v], low[v] = index, index
		stack = append(stack, v)
		on[v] = true
		for w := range graph[v] {
			if indices[w] == 0 {
				visit(w)
				low[v] = minInt(low[v], low[w])
			} else if on[w] {
				low[v] = minInt(low[v], indices[w])
			}
		}
		if low[v] == indices[v] {
			var c []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[w] = false
				c = append(c, w)
				if w == v {
					break
				}
			}
			if len(c) > 1 {
				sort.Strings(c)
				cycles = append(cycles, c)
			}
		}
	}
	for v := range graph {
		if indices[v] == 0 {
			visit(v)
		}
	}
	sort.Slice(cycles, func(i, j int) bool { return strings.Join(cycles[i], ",") < strings.Join(cycles[j], ",") })
	return cycles
}

func applyHistory(root string, files map[string]*sourceFile, cfg Config, report *Report) {
	cmd := exec.Command("git", "-C", root, "log", "-n", strconv.Itoa(cfg.HistoryCommits), "--name-only", "--pretty=format:__COMMIT__")
	cmd.Args = append(cmd.Args, "--relative", "--", ".")
	b, err := cmd.Output()
	if err != nil {
		report.Components = append(report.Components, ComponentScore{"change-locality", 0, 5, "git history unavailable"})
		report.Notes = append(report.Notes, "git history unavailable; change-coupling score omitted")
		return
	}
	counts, pairs := map[string]int{}, map[string]map[string]int{}
	matchedCommits := 0
	for _, block := range strings.Split(string(b), "__COMMIT__") {
		mods := map[string]bool{}
		s := bufio.NewScanner(strings.NewReader(block))
		for s.Scan() {
			p := filepath.ToSlash(strings.TrimSpace(s.Text()))
			if f := files[p]; f != nil && f.Module != "" {
				mods[f.Module] = true
			}
		}
		if len(mods) > 0 {
			matchedCommits++
		}
		for a := range mods {
			counts[a]++
			if pairs[a] == nil {
				pairs[a] = map[string]int{}
			}
			for z := range mods {
				if a != z {
					pairs[a][z]++
				}
			}
		}
	}
	if matchedCommits == 0 {
		report.Components = append(report.Components, ComponentScore{"change-locality", 0, 5, "no matching git history"})
		report.Notes = append(report.Notes, "no analyzed source files were found in git history; change-coupling score omitted")
		return
	}
	totalPenalty := 0
	for i := range report.Modules {
		m := &report.Modules[i]
		best := 0.0
		for p, n := range pairs[m.Name] {
			ratio := float64(n) / float64(maxInt(1, counts[m.Name]))
			if ratio >= 0.5 {
				m.ChangePartners = append(m.ChangePartners, p)
			}
			if ratio > best {
				best = ratio
			}
		}
		sort.Strings(m.ChangePartners)
		m.ChangeCoupling = round(best)
		if best > 0.75 {
			totalPenalty += 2
		} else if best > 0.5 {
			totalPenalty++
		}
	}
	score := maxInt(0, 5-totalPenalty)
	report.Components = append(report.Components, ComponentScore{"change-locality", score, 5, fmt.Sprintf("analyzed %d matching commits (up to %d)", matchedCommits, cfg.HistoryCommits)})
	report.Score += score
}

func round(v float64) float64 { return math.Round(v*1000) / 1000 }
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
