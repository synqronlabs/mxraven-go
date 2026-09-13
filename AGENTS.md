# AGENTS.md

## Core Principles

* Write idiomatic Go that is easy to read, review, test, and maintain.
* Prefer the standard library unless an external dependency provides clear, substantial value.
* Keep abstractions proportional to the problem. Do not introduce interfaces, generics, configuration layers, or helpers speculatively.
* Minimize the exported API. Every exported identifier is a compatibility commitment.
* Preserve backward compatibility unless a breaking change is intentional, documented, and released under the appropriate SemVer major version.
* Leave the codebase simpler than you found it.

## Go Style

Follow standard Go conventions and the guidance enforced by `gofmt`, `go vet`, and `golangci-lint`.

Before considering a change complete:

```sh
gofmt -w .
go vet ./...
golangci-lint run
go test ./...
```

Run additional repository-specific checks when they are configured by the project.

### golangci-lint

`golangci-lint` is a required quality gate.

* Run `golangci-lint run` for every code change.
* Treat lint failures as defects to fix, not obstacles to bypass.
* Use the repository's checked-in golangci-lint configuration as the source of truth.
* Keep CI and local lint behavior aligned.
* Pin or otherwise control the golangci-lint version used in CI so lint results do not change unexpectedly.
* Do not disable a linter globally merely to avoid fixing a local issue.
* Prefer fixing the underlying code over adding `//nolint` directives.
* Use `//nolint` only when the warning is genuinely inappropriate and the exception is intentional.
* A `//nolint` directive should name the relevant linter and explain the reason when it is not self-evident.

Prefer:

```go
//nolint:errcheck // Best-effort cleanup; the primary operation error takes precedence.
_ = file.Close()
```

over:

```go
//nolint
_ = file.Close()
```

Do not change lint configuration as an incidental part of an unrelated feature or bug fix unless the change is necessary and justified.

### Naming

* Use short, descriptive names whose meaning is clear from context.
* Follow standard Go initialism casing: `ID`, `HTTP`, `URL`, `JSON`, `API`, etc.
* Avoid package names such as `util`, `utils`, `common`, `base`, or `misc`.
* Avoid stuttering such as `http.HTTPClient` when `http.Client` is sufficient.
* Name interfaces after behavior where practical: `Reader`, `Writer`, `Closer`.
* Keep receiver names short and consistent across methods for the same type.

### Code Structure

* Prefer early returns to deeply nested conditionals.
* Keep functions focused on one responsibility.
* Prefer concrete types until an interface is genuinely required.
* Define interfaces at the point of use rather than the point of implementation when practical.
* Accept interfaces; return concrete types.
* Avoid package-level mutable state.
* Avoid `init` unless initialization genuinely cannot be expressed explicitly.
* Prefer useful zero values where practical.
* Do not add dependencies merely to save a few lines of straightforward Go.

## Public API Design

Treat every exported symbol as long-lived API.

* Keep the public surface as small as possible.
* Do not export implementation details.
* New exported APIs require documentation and tests.
* Prefer APIs that are difficult to misuse.
* Avoid unnecessary constructors when the zero value is useful.
* Avoid parameter lists containing several values of the same type when their ordering is easy to confuse; consider a configuration type or functional options when justified.
* Do not expose an interface solely so callers can mock an implementation.
* Avoid changing existing interfaces. Adding a method to an exported interface is a breaking change.
* Consider allocation behavior and ownership when designing APIs operating on slices, buffers, or large values.
* Clearly document whether returned or accepted slices, maps, pointers, and buffers may be retained or mutated.

## Documentation

Documentation is part of the API.

Every exported package, type, function, method, constant, and variable should have useful Go documentation.

Documentation should explain:

* what the API does;
* behavior not obvious from its signature;
* ownership or mutation rules;
* concurrency guarantees;
* meaningful error conditions;
* important limits or invariants.

Do not merely restate the identifier's name.

Public identifiers should use conventional Go doc comments beginning with the identifier where appropriate:

```go
// Parse parses src and returns its decoded representation.
func Parse(src []byte) (*Document, error) {
	// ...
}
```

Add runnable examples for APIs whose correct use is not obvious.

Comments should explain **why**, invariants, constraints, or surprising decisions. Do not narrate straightforward code.

## Error Handling

Follow the principle: **don't just check errors; handle them gracefully**.

Errors are values and part of a library's API. Treat their semantics with the same care as function signatures.

### Add useful context

When propagating an error across a meaningful abstraction boundary, add concise context and preserve the original error:

```go
data, err := os.ReadFile(path)
if err != nil {
	return nil, fmt.Errorf("read configuration %q: %w", path, err)
}
```

Good context describes the operation that failed. Avoid redundant wording such as `"failed to"` or `"error while"` when the surrounding error chain already communicates failure.

Prefer:

```go
return fmt.Errorf("decode response: %w", err)
```

over:

```go
return fmt.Errorf("failed to decode response because an error occurred: %w", err)
```

Do not mechanically wrap an error at every stack frame. Add context only when that layer contributes useful information.

### Preserve error identity

Use `%w` when callers may need to inspect the cause.

Use:

```go
errors.Is(err, target)
errors.As(err, &target)
```

Do not compare wrapped errors with `==` unless identity comparison is explicitly part of the contract.

Never make program behavior depend on matching `err.Error()` strings. Error strings are for humans, not control flow.

### Handle an error once

An error should normally be handled at one appropriate layer.

Do not log an error and then return the same error to a caller that will also handle or log it:

```go
// Avoid.
if err != nil {
	log.Printf("request failed: %v", err)
	return err
}
```

A reusable library generally should not log ordinary operation errors at all. Return them with sufficient context and allow the application to decide how they should be reported.

Handling an error can mean:

* recovering or retrying;
* translating it into a stable API-level error;
* adding useful context and propagating it;
* intentionally ignoring it with a documented reason;
* terminating an operation when continuing would be incorrect.

Simply writing `if err != nil` is not, by itself, adequate error handling.

### Sentinel errors

Avoid introducing sentinel errors unless callers genuinely need stable programmatic identity.

If a sentinel is part of the public API:

* document exactly when it is returned;
* keep its semantics stable;
* expect callers to use `errors.Is`;
* wrap it rather than destroying its identity when adding context.

Do not create a sentinel merely to make tests easier.

### Custom error types

Use exported custom error types sparingly. Once exposed, their structure and behavior become part of the public API.

Prefer opaque errors unless callers require structured information.

When callers need to make decisions based on an error, prefer exposing the smallest stable behavior necessary instead of leaking internal implementation details.

### Cleanup errors

Do not silently discard important errors from cleanup operations.

For example, when closing or flushing a resource can affect correctness, propagate that error appropriately:

```go
if err := w.Close(); err != nil {
	return fmt.Errorf("close writer: %w", err)
}
```

When both the primary operation and cleanup fail, preserve the information that is useful to callers. On supported Go versions, `errors.Join` may be appropriate when multiple independent errors must be retained.

### Context cancellation

Functions performing blocking or potentially long-running work should accept `context.Context` when cancellation, deadlines, or request-scoped values are genuinely relevant.

When accepted:

* make `context.Context` the first parameter;
* do not store it in a struct;
* propagate it to downstream operations;
* honor cancellation promptly where practical;
* do not replace cancellation errors with unrelated errors.

### Panics

Do not use panic for expected runtime failures or invalid external input.

Panics are reserved for programmer errors or impossible internal states where continuing would indicate a broken invariant.

Library APIs should normally return errors instead.

## Concurrency

Make concurrency ownership explicit.

* Do not start a goroutine without knowing how and when it terminates.
* Avoid goroutine leaks.
* Clearly document whether public types and methods are safe for concurrent use.
* Do not hold locks while calling arbitrary user-provided functions unless the contract explicitly requires it.
* Prefer synchronous implementations unless concurrency provides a measurable or structural benefit.
* Use channels for communication and coordination, not merely because concurrency is available.

Run race-sensitive changes with:

```sh
go test -race ./...
```

## Testing

Every behavioral change should have tests appropriate to its risk.

Prefer table-driven tests when several cases exercise the same behavior:

```go
func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "valid",
			input: "example",
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)

			if tt.wantErr && err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
		})
	}
}
```

Testing expectations:

* Test observable behavior rather than implementation details.
* Test error semantics with `errors.Is` or `errors.As` when those semantics are part of the contract.
* Do not depend on complete error strings unless the exact human-readable output is itself explicitly contractual.
* Include edge cases and failure paths.
* Add regression tests for bug fixes.
* Avoid sleeps for synchronization where deterministic coordination is possible.
* Keep tests independent and safe to run in any order.
* Use `t.TempDir()` for temporary files.
* Use `t.Cleanup()` for test cleanup where appropriate.

Use benchmarks for performance-sensitive code. Do not claim a performance improvement without benchmark evidence.

## Compatibility

This library follows Semantic Versioning.

Given a version `MAJOR.MINOR.PATCH`:

* **PATCH**: backward-compatible bug fixes and internal improvements.
* **MINOR**: backward-compatible functionality or newly exposed API.
* **MAJOR**: incompatible changes to the public API or documented behavior.

For `v1` and later, do not introduce breaking API changes in minor or patch releases.

Breaking changes include, among other things:

* removing or renaming exported identifiers;
* changing exported function or method signatures;
* adding methods to exported interfaces;
* removing accepted input forms that were documented or intentionally supported;
* changing documented semantics in incompatible ways;
* changing error behavior when callers are expected to inspect that behavior;
* changing concurrency or ownership guarantees incompatibly.

Deprecate before removing APIs whenever practical:

```go
// Deprecated: Use ParseWithOptions instead.
func ParseLegacy(src []byte) (*Document, error) {
	// ...
}
```

Deprecation alone does not make removal backward compatible.

### Pre-1.0 versions

For versions below `v1.0.0`, breaking changes may be necessary, but they must still be deliberate, documented, and reflected appropriately in the version number.

Do not use the pre-1.0 status as an excuse for unnecessary API churn.

### Go module major versions

For modules at `v2` or later, follow Go's semantic import versioning rules and use the corresponding `/vN` module path.

## Commits

Use semantic commit messages following the Conventional Commits style.

Format:

```text
<type>[optional scope][!]: <description>
```

Common types:

* `feat`: backward-compatible user-visible functionality;
* `fix`: bug fix;
* `docs`: documentation-only changes;
* `test`: test-only changes;
* `refactor`: behavior-preserving restructuring;
* `perf`: performance improvement;
* `build`: build or dependency changes;
* `ci`: CI configuration;
* `chore`: repository maintenance.

Examples:

```text
feat(parser): support streaming input
fix(reader): preserve EOF when wrapping errors
docs: clarify buffer ownership
test(parser): cover truncated input
refactor(codec): remove duplicate allocation
perf(encoder): reuse scratch buffer
```

Use imperative, concise descriptions without a trailing period.

### Breaking commits

Mark intentionally breaking changes with `!`:

```text
feat(api)!: replace Decoder.Read with Decoder.Decode
```

Include a `BREAKING CHANGE:` footer explaining the migration:

```text
feat(api)!: replace Decoder.Read with Decoder.Decode

BREAKING CHANGE: Decoder.Read has been removed. Call Decoder.Decode instead.
```

A breaking commit requires a SemVer major release once the module is stable at `v1` or later.

A `feat` normally requires a minor release. A `fix` normally requires a patch release. Commit type alone does not override SemVer: determine the release from the actual public impact.

## Dependencies

Before adding a dependency:

1. determine whether the standard library is sufficient;
2. evaluate the maintenance and compatibility cost;
3. verify that the dependency solves substantially more than a small local implementation would;
4. avoid exposing dependency-specific types through the public API unless intentional.

Keep `go.mod` and `go.sum` tidy:

```sh
go mod tidy
```

Do not upgrade unrelated dependencies as part of an otherwise focused change.

## Change Discipline

Keep changes focused.

Do not mix unrelated refactoring with functional changes unless the refactoring is necessary to implement the change safely.

When modifying existing code:

1. understand the current API contract;
2. identify compatibility constraints;
3. make the smallest coherent change;
4. add or update tests;
5. update documentation;
6. format the code;
7. run `go vet ./...`;
8. run `golangci-lint run`;
9. run the relevant test suite;
10. consider whether the change affects SemVer.

Do not silently alter public behavior because an alternative appears cleaner.

## Definition of Done

A change is complete when:

* the implementation is idiomatic and appropriately simple;
* error cases are handled deliberately and preserve useful context;
* exported APIs are documented;
* tests cover the new or changed behavior;
* `gofmt` produces no changes;
* `go vet ./...` succeeds;
* `golangci-lint run` succeeds without unjustified suppressions;
* `go test ./...` succeeds;
* `go test -race ./...` succeeds when concurrency is affected;
* repository-specific checks succeed;
* public compatibility has been considered;
* the required SemVer impact is understood;
* the commit message accurately describes the change using semantic commit conventions.

