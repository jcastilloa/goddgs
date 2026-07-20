---
name: go-code-simplification
description: Simplifies Go code for clarity without changing behavior. Use when Go code works but is harder to read, maintain, test, or extend than necessary.
---

# Go Code Simplification

## Overview

Simplify Go code by reducing accidental complexity while preserving exact behavior.

The goal is not fewer lines. The goal is code that is easier to read, review, debug, test, and safely change.

A simplification must pass this test:

> Would a Go developer familiar with this codebase understand the new version faster than the old one?

## When to Use

Use this skill when:

- A feature works and tests pass, but the implementation feels heavier than needed
- Code review flags readability, duplication, nesting, unclear naming, or unclear error paths
- A function has grown too large or handles several responsibilities
- Logic is deeply nested or scattered across files
- Similar validation, transformation, logging, or error-handling logic is duplicated
- A previous change introduced temporary complexity

Do not use this skill when:

- After Step 1 you still do not understand the relevant behavior, API contract, and risks well enough to preserve them
- The code is already clear and idiomatic
- The code is performance-critical and the simpler version may change allocations, latency, ordering, or concurrency behavior
- The module is about to be rewritten anyway
- The change would mix broad refactoring with feature or bug-fix work

## Core Principles

### 1. Preserve Behavior Exactly

Do not change what the code does. Change only how clearly it expresses it.

Before each simplification, check that the new version preserves:

```text
- Return values
- Error identity and wrapping (errors.Is, errors.As, sentinels, direct equality)
- Side effects and their ordering
- nil vs empty slice/map behavior, and typed nil inside interfaces
- Concurrency behavior, goroutine lifetime, and channel closing
- context.Context cancellation behavior
- Exported API (names, signatures, receivers)
- defer ordering
- Externally observable ordering; do not replace stable/sorted output with map iteration
- JSON, YAML, database, reflection, and tag-based behavior
- Existing tests pass without modification
```

This list is the canonical reference. Later steps refer back to it rather than repeating it.

If unsure about any of these, do not simplify yet.

### 2. Follow the Project's Go Style

Simplification means becoming more consistent with the codebase, not imposing personal style.

Before changing code:

```text
- Read project conventions: README, CONTRIBUTING, CLAUDE.md, AGENTS.md, docs
- Inspect neighboring packages
- Match naming, error handling, logging, test style, and package layout
- Run gofmt / goimports
- Run the existing test, lint, and vet commands (e.g. go vet, staticcheck, golangci-lint)
```

Prefer idiomatic Go, but respect established local conventions unless they are clearly harmful.

### 3. Prefer Plain Go Over Clever Go

Go usually favors simple control flow, explicit error handling, and small focused functions.

Prefer:

- guard clauses over deep nesting
- clear loops over clever generic helpers
- explicit error handling over hidden control flow
- small interfaces at the consumer side
- concrete types unless an interface buys something real
- simple constructors over elaborate factories
- boring code over impressive code

Do not compress code just to reduce line count.

### 4. Keep the Right Abstractions

Removing abstraction can simplify code, but only when the abstraction adds no value.

Keep abstractions that:

- define a meaningful boundary
- make tests easier without distorting production code
- isolate external systems
- represent a real domain concept
- prevent duplication across meaningful call sites

Remove or inline abstractions that:

- wrap one function without adding meaning
- exist only "in case we need it later"
- create interfaces with a single implementation and no testing or boundary value
- obscure simple logic behind unnecessary indirection

### 5. Keep Refactors Scoped

Default to simplifying recently touched code.

Avoid drive-by refactors unless explicitly asked. Large unrelated cleanups create noisy diffs and make regressions harder to trace.

## Process

### Step 1: Understand Before Touching

Before changing code, answer:

```text
- What is this package responsible for?
- Is this symbol exported? Who imports it?
- What are the important error cases, and how do callers compare errors?
- Are nil values, ordering, or concurrency observable by callers?
- Are there tests, examples, or golden files defining behavior?
- Why might the code be written this way?
```

Check git history when something looks odd. Do not remove a "weird" construct until you know why it exists.

### Step 2: Identify Simplification Opportunities

#### Structural complexity

| Pattern | Signal | Go-oriented simplification |
|---|---|---|
| Deep nesting | Many nested `if`s | Use guard clauses and early returns |
| Long function | Multiple responsibilities | Extract focused helpers |
| Repeated validation | Same checks in several places | Extract a predicate or validation function |
| Repeated error decoration | Same wrapping/logging pattern | Extract a helper only if it improves clarity |
| Large switch doing unrelated work | Mixed responsibilities | Split by responsibility or type |
| Setup mixed with business logic | Hard-to-test function | Move setup to caller, constructor, or helper |

#### Naming and readability

| Pattern | Signal | Simplification |
|---|---|---|
| Vague names | `data`, `result`, `tmp`, `val` | Rename to domain terms |
| Over-abbreviation | `usr`, `cfg`, `svc` everywhere | Prefer full words unless conventional |
| Misleading names | `Get` mutates state | Rename to reflect behavior |
| Stuttered names | `user.UserService` | Use package-aware names |
| Comment explains obvious code | `// increment count` | Delete it |
| Comment explains intent | `// retry because provider may return stale 404` | Keep it |

Go accepts short names in small scopes:

```go
for _, user := range users {
    // ...
}
```

But avoid short names when the scope is large or the meaning is not obvious.

#### Redundancy

| Pattern | Signal | Simplification |
|---|---|---|
| Duplicated logic | Same block in several places | Extract helper |
| Unused code | Dead functions, vars, branches | Remove after confirming |
| Wrapper with no value | Function only forwards args | Inline it |
| Interface with one implementation | No boundary/test value | Use concrete type |
| Config struct with one field | Adds ceremony only | Pass the value directly, unless future shape is real |
| Generic helper used once | Harder than loop | Inline the loop |
| `interface{}` in new code | Pre-1.18 idiom | Use `any` (Go 1.18+) if project style agrees |

### Step 3: Apply Changes Incrementally

Make one simplification at a time.

```text
FOR EACH SIMPLIFICATION:
1. Make the change
2. Run the smallest useful test
3. If tests pass, continue or commit
4. If tests fail, revert and reconsider
```

Prefer small, reviewable diffs. Submit broad refactors separately from feature or bug-fix changes.

If a refactor would touch hundreds of lines, consider automation or split the work.

### Step 4: Verify the Result

After simplifying, confirm the new version preserves every item listed in **Core Principle 1**, and then ask:

- Is the new version genuinely easier to understand?
- Is the diff focused and reviewable?
- Does the result match neighboring Go style?

If the "simplified" version is harder to understand or review, revert.

## Go-Specific Guidance

### Guard Clauses Are Usually Clearer

```go
// Before
func Process(ctx context.Context, user *User) error {
    if user != nil {
        if user.Active {
            if err := validate(user); err == nil {
                return save(ctx, user)
            } else {
                return err
            }
        }
        return ErrInactiveUser
    }
    return ErrNilUser
}

// After
func Process(ctx context.Context, user *User) error {
    if user == nil {
        return ErrNilUser
    }
    if !user.Active {
        return ErrInactiveUser
    }
    if err := validate(user); err != nil {
        return err
    }
    return save(ctx, user)
}
```

### Keep Error Behavior Intact

Error identity and wrapping are observable behavior. Only simplify error handling when the resulting behavior is intentionally identical.

```go
// Before — adds context via %w
if err := repo.Save(ctx, user); err != nil {
    return fmt.Errorf("save user %s: %w", user.ID, err)
}
return nil

// Bad: loses wrapping and context
return repo.Save(ctx, user)
```

Safe simplification when there is no added context:

```go
// Before
err := repo.Save(ctx, user)
if err != nil {
    return err
}
return nil

// After
return repo.Save(ctx, user)
```

Do not apply the above when the function adds context, logging, metrics, cleanup, or error conversion.

### Preserve nil vs Empty Slices

```go
func Names(users []User) []string {
    if users == nil {
        return nil
    }

    names := make([]string, 0, len(users))
    for _, user := range users {
        names = append(names, user.Name)
    }
    return names
}
```

Do not "simplify" this to always return `[]string{}` unless callers cannot observe the difference. JSON output, tests, and API contracts often can.

### Preallocate When It Clarifies Intent

Preallocate only when the resulting nil/empty behavior is still correct for the caller contract.

```go
func IDs(users []User) []string {
    if users == nil {
        return nil
    }

    ids := make([]string, 0, len(users))
    for _, user := range users {
        ids = append(ids, user.ID)
    }
    return ids
}
```

If the previous code could return a nil slice, do not replace it with a non-nil empty slice unless that behavior is intentionally acceptable.

### Prefer Clear Loops Over Clever Helpers

Go does not have built-in `map`, `filter`, or `reduce`. Do not introduce generic helpers (or generics in general) just to imitate another language.

```go
active := make([]User, 0, len(users))
for _, user := range users {
    if user.Active {
        active = append(active, user)
    }
}
```

This is usually better than a custom `Filter[T any](s []T, pred func(T) bool) []T` unless the helper is already established in the project and genuinely improves readability. Introduce generics only when they remove real duplication across several concrete types.

### Avoid Unnecessary Interfaces; Accept Interfaces, Return Concrete Types

Useful when `Service` needs a boundary for tests or multiple implementations:

```go
type UserGetter interface {
    GetUser(ctx context.Context, id string) (*User, error)
}

type Service struct {
    users UserGetter
}

func NewService(users UserGetter) *Service {
    return &Service{users: users}
}
```

But avoid interfaces with a single implementation and no meaningful boundary — a concrete type is simpler. As a default heuristic, accept interfaces in parameters and return concrete types from constructors; returning an interface hides useful behavior and makes code harder to navigate.

### Be Careful With Struct Tags

Renaming a field changes serialization unless its tag pins the external name. When simplifying, treat tags as part of the public contract:

```go
type User struct {
    FullName string `json:"full_name"` // renaming the field to `Name` alone does not change JSON output
}
```

Check JSON, YAML, DB, and validation tags before renaming or removing fields. Renaming an exported field is also an API change, even when JSON output stays the same because a tag pins the external name.

### Keep context.Context Explicit

Do not hide context in structs just to reduce parameters.

```go
// Good
func (s *Service) GetUser(ctx context.Context, id string) (*User, error)

// Avoid
type Service struct {
    ctx context.Context
}
```

Context should usually be passed through call chains explicitly.

### Be Careful With defer in Loops

```go
// Risky: defers all closes until the function returns
for _, name := range names {
    f, err := os.Open(name)
    if err != nil {
        return err
    }
    defer f.Close()
    // ...
}
```

Prefer extracting the loop body:

```go
for _, name := range names {
    if err := processFile(name); err != nil {
        return err
    }
}

func processFile(name string) error {
    f, err := os.Open(name)
    if err != nil {
        return err
    }
    defer f.Close()
    // ...
    return nil
}
```

### Avoid Hidden Goroutine Complexity

Do not "simplify" concurrent code by hiding synchronization in helpers unless it makes lifecycle clearer.

Check:

```text
- Who owns closing the channel?
- How does cancellation work?
- Can goroutines leak?
- Are errors propagated?
- Is ordering important?
```

Prefer straightforward concurrency over compact but subtle abstractions.

### Simplify Boolean Parameters Carefully

```go
// Hard to read
SendEmail(user, true, false)
```

Prefer named variants, or a small options struct when several flags are needed:

```go
SendWelcomeEmail(user)
SendPasswordResetEmail(user)

SendEmail(user, EmailOptions{Urgent: true, Track: false})
```

Avoid defaulting to the functional options pattern unless the project already uses it or the configuration is genuinely extensible.

### Delete Dead Code Aggressively, But Safely

Remove unused functions, unreachable branches, commented-out code, and unused struct fields — but only after confirming they are not serialized, reflected, tagged, or part of the public API.

### Simplify Tests Too

Prefer table-driven tests when cases share the same structure:

```go
func TestIsValid(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  bool
    }{
        {name: "empty", input: "", want: false},
        {name: "short", input: "abc", want: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := IsValid(tt.input); got != tt.want {
                t.Fatalf("IsValid(%q) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}
```

But do not force table tests when a single explicit test is clearer.

## Common Go Simplifications

Minor patterns that most linters (`gosimple`, `staticcheck`) already flag:

- **Redundant boolean return:** `if cond { return true }; return false` → `return cond`.
- **Unnecessary temporary variable:** drop it unless the name documents a non-obvious expression.
- **Redundant error plumbing:** `err := f(); if err != nil { return err }; return nil` → `return f()` (only when no context is added).

Higher-value patterns:

### Extract Predicate

```go
// Before
if user.Active && !user.Deleted && user.EmailVerified {
    // ...
}

// After
if canReceiveEmail(user) {
    // ...
}

func canReceiveEmail(user User) bool {
    return user.Active && !user.Deleted && user.EmailVerified
}
```

Worthwhile when the condition represents a domain concept or appears more than once.

### Replace Speculative Abstraction

```go
// Before
type UserFactory struct{}

func (f UserFactory) New(name string) User {
    return User{Name: name}
}

// After
func NewUser(name string) User {
    return User{Name: name}
}
```

Use factories or builders only when construction is genuinely complex.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "Fewer lines is simpler" | A compact but subtle one-liner is worse than clear multi-line code. |
| "This interface makes it cleaner" | Interfaces simplify only when they express a useful boundary. |
| "The helper might be useful later" | Speculative helpers are complexity until they are actually needed. |
| "It is only a small error wrapping change" | Error identity and wrapping are observable behavior in Go. |
| "nil and empty slices are basically the same" | They can differ in JSON, tests, reflection, and API contracts. |
| "I will clean up nearby code too" | Unrelated cleanup creates noisy diffs and increases regression risk. |
| "The abstraction hides concurrency details" | Hidden goroutine/channel lifecycles are often harder to reason about. |

## Red Flags

- Tests need to change after a "simplification"
- Error wrapping or sentinel errors changed accidentally
- `nil` and empty slices/maps treated as interchangeable without checking callers
- An interface was introduced "for cleanliness" with only one implementation
- A generic helper replaced a simple loop and made the code harder to read
- A helper was inlined even though its name explained an important concept
- Concurrency code became shorter but lifecycle became less obvious
- `context.Context` was hidden in a struct to reduce parameters
- Struct field renamed or removed without checking tags and reflection users
- Public API changed as part of an internal cleanup
- Refactoring touches unrelated packages without a clear reason
- gofmt, goimports, tests, or project lint were not run

## Verification Checklist

After simplifying:

- [ ] `gofmt` / `goimports` applied
- [ ] `go test ./...` passes
- [ ] `go vet ./...` and project lint (`staticcheck`, `golangci-lint`) pass, if used
- [ ] All items in **Core Principle 1** still hold (behavior, errors, nil/empty, concurrency, context, API, tags)
- [ ] Diff is small and reviewable
- [ ] No unrelated cleanup mixed in
- [ ] Code matches neighboring Go style
