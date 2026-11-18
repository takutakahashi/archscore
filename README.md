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

## Scoring (100 points)

| Component | Points | Deterministic input |
|---|---:|---|
| Boundaries | 35 | forbidden resolved dependency edges |
| Acyclicity | 25 | strongly connected module components |
| Coupling | 15 | module fan-out above `maxFanOut` |
| Cohesion | 10 | internal / all resolved module edges |
| Module coverage | 10 | source files assigned to a module |
| Change locality | 5 | cross-module co-change in recent commits |

Boundary violations and cycles fail the gate independently of the numeric
score. Every violation includes its source file, line, import and module pair.

## Scope and limitations

- Go internal imports are resolved from the root `go.mod` module path.
- TypeScript relative imports are resolved. `tsconfig.json` path aliases and
  package workspace aliases are not yet resolved.
- Generated code, tests, vendored dependencies and other files should be
  excluded explicitly when they should not affect the score.
- Cohesion and change coupling are indicators, not universal design truths.
  The declared dependency policy is the authoritative architectural intent.
