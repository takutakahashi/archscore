# archscore

`archscore` is a deterministic architecture fitness-function CLI for Go and
TypeScript repositories. It builds a module dependency graph from source code,
checks declared boundaries, finds module cycles, measures coupling/cohesion,
and optionally uses Git history to estimate change coupling.

## Why Go?

Go keeps the analyzer small and deployable as a single binary. Its standard
library provides an exact Go import parser and graph analysis needs no runtime
dependencies. TypeScript dependency syntax is deliberately limited to static
`import`, `export ... from`, `require()`, and literal dynamic `import()` forms.

## Use

```sh
cp archscore.example.json archscore.json
go run ./cmd/archscore -root /path/to/repository -config archscore.json
```

Exit codes are `0` for a passing architecture, `2` when a forbidden dependency
or cycle trips the gate, and `1` for configuration or analysis errors. Output is
JSON so CI can archive, compare, or render it.

## Discover boundary candidates

`discover` proposes boundaries before an `archscore.json` policy exists:

```sh
go run ./cmd/archscore discover \
  -root /path/to/repository \
  -min-clusters 8 \
  -max-clusters 30 \
  -history 200 \
  -max-boundary-size 12 \
  -max-boundary-files 100
```

The atomic unit is a Go package or TypeScript source directory. Deterministic
agglomerative clustering combines three normalized signals:

- static dependency affinity: 50%
- Git co-change Jaccard affinity: 40%
- path vocabulary affinity: 10%

Co-change evidence requires at least three shared commits by default. Generic
path words such as `internal`, `pkg`, and `src` are ignored. Equal merge scores
are resolved by lexical order, so the same source and history produce the same
result. The output contains coarse, middle, and fine candidates with evidence
for every suggested boundary. Candidates are starting points for architectural
judgment, not automatically approved policies.

Each suggested boundary includes ready-to-use `paths` patterns scoped to files
directly inside its package directories. This avoids turning a parent package
such as `internal` into `internal/**` and accidentally absorbing every child
package.

Modules can declare architecture roles and local coupling thresholds. A
`composition-root` is not penalized for fan-out because wiring dependencies is
its responsibility:

```json
{
  "name": "commands",
  "paths": ["cmd/**"],
  "role": "composition-root",
  "maxFanOut": 0,
  "mayDependOn": ["application", "infrastructure"]
}
```

Change coupling is evaluated only after `minModuleChanges` (default 5), and a
relationship needs `minCochange` shared commits (default 3). Insufficient
history receives a neutral score with reduced report confidence rather than a
penalty. Single-package Go modules also receive neutral cohesion because files
inside a Go package do not import one another.

Test sources (`*_test.go`, `*.test.ts(x)`, and `*.spec.ts(x)`) are excluded from
production scoring by default. Set `includeTests` to `true` to audit the
combined production-and-test dependency graph.

Discovery raises its maximum cluster count when necessary to fit all source
units within `max-boundary-size`; disable this with `-auto-scale=false`. The
default fan-out threshold grows with the square root of the module count, and
coupling is scored by the proportion of modules above that threshold. A
module-level `maxFanOut` remains an explicit, non-scaled override.

A `composition-root` with inbound module dependencies receives a warning. This
usually means reusable code and application wiring share a declared boundary.

Declare a module's public contract with `publicPaths`. Cross-module imports may
target those paths, but imports of any other path in that module are reported
as `private-dependency` violations. The analyzer counts exported Go and
TypeScript declarations in the public paths and applies `maxPublicSymbols`:

```json
{
  "name": "notifications",
  "paths": ["internal/capabilities/notifications/**"],
  "publicPaths": ["internal/capabilities/notifications/public/**"],
  "maxPublicSymbols": 10,
  "maxFanOut": 3,
  "mayDependOn": ["platform-storage"]
}
```

Use a saved report as a CI baseline. The comparison detects score loss, new
violations or cycles, unassigned files, public API growth, and confidence loss:

```sh
archscore compare \
  -baseline architecture-baseline.json \
  -current current.json \
  -fail-on-regression
```

Baselines must use the same report version. Regenerate version 1 baselines
after upgrading to the evidence-normalized version 2 scoring model.

## Scoring (100 points)

| Component | Points | Deterministic input |
|---|---:|---|
| Boundaries | 30 | forbidden and private dependency edges |
| Acyclicity | 20 | strongly connected module components |
| Public contract | 10 | exported symbols under declared public paths |
| Coupling | 15 | module fan-out above `maxFanOut` |
| Cohesion | 10 | internal / all resolved module edges |
| Module coverage | 10 | source files assigned to a module |
| Change locality | 5 | commits whose analyzed changes are at least 80% contained |

Acyclicity is proportional to the share of active modules outside strongly
connected components. A cycle spanning many modules therefore costs more than
a small local cycle, regardless of how many strongly connected components are
present.

For every cycle, `cycleDiagnostics` ranks up to five break candidates. Each
candidate shows its inbound/outbound dependency pairs and the largest cycle
that would remain if that boundary were split out. This is a diagnosis aid,
not an instruction to delete the module.

Reports use scoring model version 2. `score` is normalized over evaluable
points only, so missing evidence is removed from both the numerator and the
denominator. `nominalScore` retains the unnormalized sum for diagnostics,
`evaluatedPoints` shows how many of the 100 possible points had evidence, and
`confidence` is `evaluatedPoints / 100`. Each component reports the same
information.

Boundary compliance is scored by the share of observed directed module pairs
that violate policy. Repeating the same forbidden import in many files remains
visible in the violation list but does not multiply the architectural breadth
of the violation.

Public-contract scoring only considers modules declaring `publicPaths`.
Cohesion excludes single-package or edge-free modules and weights the remaining
modules by dependency edges. Change locality only scores modules with enough
history and weights their result by the number of observed changes, so a module
with five commits does not count as much as one with fifty.

Use `"policyMode": "observational"` for exploratory configurations that allow
dependencies broadly. Violations and cycles are still reported, but boundary
points are excluded from the observed score. The default is `enforced`.

Boundary violations and cycles fail the gate independently of the numeric
score. Every violation includes its source file, line, import and module pair.

## Scope and limitations

- Go internal imports are resolved from the root `go.mod` module path.
- TypeScript relative imports are resolved. `tsconfig.json` path aliases and
  package workspace aliases are not yet resolved.
- Tests are excluded by default. Generated code and other repository-specific
  sources should be excluded explicitly when they should not affect the score.
- Build archscore with the latest stable Go release. Syntax unsupported by the
  build toolchain is reported as an analysis error rather than approximated.
- Export counting measures declarations, not the full type-level API surface
  such as every exported struct field.
- Cohesion and change locality are indicators, not universal design truths.
  The declared dependency policy is the authoritative architectural intent.
- Boundary discovery uses directory-level TypeScript units and does not yet
  infer workspace aliases or semantic domain vocabulary.
