# Linter Guide - Detailed Fix Patterns

Comprehensive reference for fixing issues from common golangci-lint linters.

## Table of Contents

- [err113 - Static Errors](#err113---static-errors)
- [errcheck - Error Checking](#errcheck---error-checking)
- [errname - Error Naming](#errname---error-naming)
- [errorlint - Error Handling](#errorlint---error-handling)
- [gosec - Security](#gosec---security)
- [modernize - Modern Go](#modernize---modern-go)
- [staticcheck - Static Analysis](#staticcheck---static-analysis)
- [gci - Import Ordering](#gci---import-ordering)
- [gofmt / gofumpt - Formatting](#gofmt--gofumpt---formatting)
- [golines - Line Length](#golines---line-length)

---

## err113 - Static Errors

**Purpose**: Enforce use of wrapped static errors instead of dynamic `fmt.Errorf()` calls.

### Pattern 1: Simple Error Message
```go
// Before
return fmt.Errorf("connection failed")

// After - at package level
var errConnectionFailed = errors.New("connection failed")

// At call site
return errConnectionFailed
```

### Pattern 2: Error with Context
```go
// Before
return fmt.Errorf("workload %q not found", name)

// After - at package level
var errWorkloadNotFound = errors.New("workload not found")

// At call site - wrap with context
return fmt.Errorf("%w: %q", errWorkloadNotFound, name)
```

### Pattern 3: Multiple Dynamic Values
```go
// Before
return fmt.Errorf("timeout waiting for %s/%s", namespace, name)

// After
var errTimeout = errors.New("timeout waiting for resource")

return fmt.Errorf("%w: %s/%s", errTimeout, namespace, name)
```

### Pattern 4: In Test Files
For test files, consider whether static errors are needed. Options:
1. **nolint directive** if error is test-specific:
   ```go
   return fmt.Errorf("simulated error") //nolint:err113
   ```
2. **Define test errors** if reused:
   ```go
   var errSimulatedDelete = errors.New("simulated delete error")
   ```

### Import Required
```go
import "errors"
```

---

## errcheck - Error Checking

**Purpose**: Ensure error return values are handled.

### Pattern 1: Must Handle Error
```go
// Before
file.Close()

// After
if err := file.Close(); err != nil {
    return fmt.Errorf("closing file: %w", err)
}
```

### Pattern 2: Defer with Error
```go
// Before
defer file.Close()

// After - option 1: named return
func doSomething() (err error) {
    file, err := os.Open(path)
    if err != nil {
        return err
    }
    defer func() {
        if cerr := file.Close(); cerr != nil && err == nil {
            err = cerr
        }
    }()
    // ...
}

// After - option 2: log on failure (for cleanup)
defer func() {
    if err := file.Close(); err != nil {
        log.Printf("failed to close file: %v", err)
    }
}()
```

### Pattern 3: Known-Safe Ignores
Some functions rarely fail when used correctly. Explicitly ignore:
```go
// Writing to stdout/stderr (rarely fails)
_ = fmt.Fprintf(os.Stdout, "message\n")

// Closing read-only files
_ = file.Close()

// Environment in tests
_ = os.Setenv("KEY", "value")
```

### Pattern 4: Flag Setting in Tests
```go
// Before
cmd.Flags().Set("config", path)

// After
if err := cmd.Flags().Set("config", path); err != nil {
    t.Fatalf("failed to set flag: %v", err)
}

// Or if truly safe to ignore in test setup
_ = cmd.Flags().Set("config", path)
```

---

## errname - Error Naming

**Purpose**: Sentinel errors should follow `errXxx` naming convention.

### Pattern
```go
// Before
var buildErr error
var NotFoundError = errors.New("not found")
var ErrTimeout error  // This is already correct

// After
var errBuild error
var errNotFound = errors.New("not found")
```

### Exported vs Unexported
- **Unexported** (package-internal): `errXxx`
- **Exported** (public API): `ErrXxx`

```go
// Unexported - use in same package
var errBuild = errors.New("build failed")

// Exported - part of public API
var ErrNotFound = errors.New("not found")
```

---

## errorlint - Error Handling

**Purpose**: Ensure proper error wrapping and type assertions for wrapped errors.

### Pattern 1: Type Assertion
```go
// Before
if exitErr, ok := err.(*exec.ExitError); ok {
    // use exitErr
}

// After
var exitErr *exec.ExitError
if errors.As(err, &exitErr) {
    // use exitErr
}
```

### Pattern 2: Error Comparison
```go
// Before
if err == io.EOF {

// After
if errors.Is(err, io.EOF) {
```

### Pattern 3: Switch on Error Type
```go
// Before
switch e := err.(type) {
case *os.PathError:
    // handle
}

// After
var pathErr *os.PathError
if errors.As(err, &pathErr) {
    // handle
}
```

### Import Required
```go
import "errors"
```

---

## gosec - Security

**Purpose**: Identify security vulnerabilities.

### G115 - Integer Overflow
```go
// Before
value := uint32(signedInt)

// After
if signedInt < 0 || signedInt > math.MaxUint32 {
    return fmt.Errorf("value %d out of uint32 range", signedInt)
}
value := uint32(signedInt)
```

### G204 - Command Injection
```go
// Before
cmd := exec.Command(userInput)

// After - validate input
allowedCommands := map[string]bool{"ls": true, "cat": true}
if !allowedCommands[cmdName] {
    return fmt.Errorf("command not allowed: %s", cmdName)
}
cmd := exec.Command(cmdName, args...)

// Or use absolute paths
cmd := exec.Command("/usr/bin/ls", args...)
```

### G301 - Directory Permissions
```go
// Before
os.MkdirAll(dir, 0755)

// After
os.MkdirAll(dir, 0750)  // No world-readable
```

### G306 - File Permissions
```go
// Before
os.WriteFile(path, data, 0644)

// After
os.WriteFile(path, data, 0600)  // Owner only
```

### G401/G501 - Weak Crypto
```go
// Before
import "crypto/md5"
hash := md5.Sum(data)

// After
import "crypto/sha256"
hash := sha256.Sum256(data)
```

### G104 - Unhandled Errors (see errcheck)

---

## modernize - Modern Go

**Purpose**: Use modern Go idioms (Go 1.21+/1.22+).

### rangeint - Range Over Integer (Go 1.22+)
```go
// Before
for i := 0; i < count; i++ {
    // use i
}

// After
for i := range count {
    // use i
}

// If i unused
for range count {
    // body
}
```

### forvar - Loop Variable Capture (Go 1.22+)
```go
// Before (needed pre-1.22)
for _, item := range items {
    item := item  // capture
    go process(item)
}

// After (1.22+ captures automatically)
for _, item := range items {
    go process(item)
}
```

### any - Replace interface{}
```go
// Before
func process(data interface{}) interface{} {}
var m map[string]interface{}

// After
func process(data any) any {}
var m map[string]any
```

### mapsloop - Use maps.Copy (Go 1.21+)
```go
// Before
for k, v := range src {
    dst[k] = v
}

// After
import "maps"
maps.Copy(dst, src)
```

### minmax - Use min/max Builtins (Go 1.21+)
```go
// Before
count := opts.Count
if count < 1 {
    count = 1
}

// After
count := max(opts.Count, 1)
```

---

## staticcheck - Static Analysis

### ST1023 - Redundant Type
```go
// Before
var name string = ""
var count int = 0

// After
var name = ""
var count = 0
// Or: name := ""
```

### SA4000 - Identical Expressions
```go
// Before (always equals 0)
result := x - x

// After - fix the logic error
result := x - y  // probably meant different variable
```

### SA1019 - Deprecated
```go
// Follow deprecation notice to use replacement API
```

### SA4006 - Unused Value
```go
// Before
x = computeValue()
x = otherValue()  // first assignment unused

// After
_ = computeValue()  // if side effects needed
x = otherValue()
```

---

## gci - Import Ordering

**Purpose**: Enforce consistent import grouping and ordering.

### Standard Order
```go
import (
    // 1. Standard library
    "context"
    "fmt"

    // 2. Third-party packages
    "github.com/spf13/cobra"
    "k8s.io/client-go/kubernetes"

    // 3. Local packages
    "github.com/opdev/virtwork/internal/config"
)
```

### Auto-Fix
gci usually provides `SuggestedFixes` - apply them directly.

---

## gofmt / gofumpt - Formatting

**Purpose**: Standard Go formatting.

### Auto-Fix
These always have `SuggestedFixes`. Apply directly or run:
```bash
gofmt -w file.go
gofumpt -w file.go
```

---

## golines - Line Length

**Purpose**: Keep lines under configured length (default 120).

### Pattern 1: Function Signature
```go
// Before
func ProcessItems(ctx context.Context, client Client, items []Item, options Options) (Result, error) {

// After
func ProcessItems(
    ctx context.Context,
    client Client,
    items []Item,
    options Options,
) (Result, error) {
```

### Pattern 2: Struct Literal
```go
// Before
return &Config{Name: name, Value: value, Options: options, Enabled: true}

// After
return &Config{
    Name:    name,
    Value:   value,
    Options: options,
    Enabled: true,
}
```

### Auto-Fix
golines provides `SuggestedFixes`. Apply directly or run:
```bash
golines -w file.go
```

---

## Quick Reference: Severity Levels

| Severity | Linters | Priority |
|----------|---------|----------|
| high | gosec (G115, G203, etc.) | Fix immediately |
| medium | gosec (G301, G306) | Fix soon |
| "" (default) | All others | Fix as convenient |

## When to Use nolint

Use `//nolint:linter` sparingly when:
1. False positive
2. Intentional deviation with good reason
3. Test-specific code

Always include reason:
```go
//nolint:gosec // G204: command is hardcoded, not user input
cmd := exec.Command(binaryPath, args...)
```
