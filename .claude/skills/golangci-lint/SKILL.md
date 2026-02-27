---
name: golangci-lint
description: This skill should be used when the user asks to "parse golangci-lint output", "fix lint errors", "review lint report", "analyze golangci-lint JSON", "fix go lint issues", "apply lint suggestions", "run golangci-lint"
version: 0.1.0
---

# golangci-lint Skill

Parse, analyze, and fix issues from golangci-lint JSON reports.

## Overview

This skill helps you:
1. Parse golangci-lint JSON output and present issues in a structured way
2. Fix lint issues interactively (one at a time) or in batch (by linter)
3. Apply suggested fixes when available
4. Validate fixes by running tests

## Usage

To generate a JSON report:
```bash
golangci-lint run --output-format json > tmp/golangci-lint-report.json 2>&1
```

Common report locations:
- `tmp/golangci-lint-report.json`
- `golangci-lint-report.json`

## JSON Structure Reference

### Top-Level Object
```json
{
  "Issues": [...],
  "Report": { "Linters": [...] }
}
```

### Issue Object
| Field | Type | Description |
|-------|------|-------------|
| `FromLinter` | string | Linter name (e.g., "err113", "gosec") |
| `Text` | string | Error message |
| `Severity` | string | "" (default), "medium", or "high" |
| `SourceLines` | []string | Source code lines at issue location |
| `Pos` | object | Position: `{Filename, Offset, Line, Column}` |
| `SuggestedFixes` | []object | Optional auto-fix data |
| `ExpectNoLint` | bool | Whether nolint was expected |
| `ExpectedNoLintLinter` | string | Expected nolint linter |

### SuggestedFix Object
```json
{
  "Message": "Description of fix",
  "TextEdits": [
    {
      "Pos": 1234,      // byte offset start
      "End": 1290,      // byte offset end
      "NewText": "..."  // base64-encoded replacement text (null = delete)
    }
  ]
}
```

## Fix Modes

### Interactive Mode (Default)
Review and fix issues one at a time with full context:
1. Show issue details (linter, file, line, message)
2. Display source context
3. Present fix options:
   - Apply suggested fix (if available)
   - Generate fix based on linter rules
   - Skip this issue
   - Stop fixing

### Batch Mode
Fix all issues from a specific linter at once:
1. Group issues by linter
2. User selects which linter to address
3. Apply fixes for all issues from that linter
4. Show summary of changes

## Fix Workflow with Validation

```
1. Check if tests exist for affected code
   └─ Look for *_test.go files in same package

2. Apply fix
   └─ Use suggested fix OR generate fix based on linter rules

3. Run tests to validate
   └─ go test ./path/to/package/...

4. If tests fail:
   └─ Prompt user:
      - "Proceed anyway" (keep fix)
      - "Revert and fix manually" (undo fix)
```

## Fix Strategies by Linter

### err113 - Static Error Definitions
**Problem**: Dynamic errors with `fmt.Errorf()` without wrapping

**Fix**: Define static sentinel errors and wrap them
```go
// Before
return fmt.Errorf("workload %q not found", name)

// After
var errWorkloadNotFound = errors.New("workload not found")
// ...
return fmt.Errorf("%w: %q", errWorkloadNotFound, name)
```

### errcheck - Unchecked Error Returns
**Problem**: Error return values not checked

**Fix Options**:
1. Check the error:
```go
// Before
auditor.Close()

// After
if err := auditor.Close(); err != nil {
    // handle or log
}
```

2. Explicitly ignore (for known-safe calls like `fmt.Fprintf` to stdout):
```go
_ = fmt.Fprintf(out, "message")
```

### errname - Sentinel Error Naming
**Problem**: Sentinel error names don't follow `errXxx` convention

**Fix**: Rename to `errXxx` format
```go
// Before
var buildErr error

// After
var errBuild error
```

### errorlint - Error Type Assertions
**Problem**: Direct type assertion on errors fails with wrapped errors

**Fix**: Use `errors.As()`:
```go
// Before
if exitErr, ok := err.(*exec.ExitError); ok {

// After
var exitErr *exec.ExitError
if errors.As(err, &exitErr) {
```

### gosec - Security Issues
**Problem**: Various security concerns

**Fixes by code**:
- **G115 (integer overflow)**: Add bounds checking
```go
// Before
Cores: uint32(opts.CPUCores)

// After
if opts.CPUCores < 0 || opts.CPUCores > math.MaxUint32 {
    return nil, errors.New("CPU cores out of range")
}
Cores: uint32(opts.CPUCores)
```

- **G301 (directory permissions)**: Use 0750 or less
```go
os.MkdirAll(dir, 0o750)  // instead of 0o755
```

- **G306 (file permissions)**: Use 0600 or less
```go
os.WriteFile(path, data, 0o600)  // instead of 0644
```

- **G204 (command injection)**: Validate inputs or use allowlist

### modernize - Modern Go Idioms
**rangeint** - Use range over int (Go 1.22+):
```go
// Before
for i := 0; i < count; i++ {

// After
for i := range count {
```

**forvar** - Remove unnecessary loop variable capture (Go 1.22+):
```go
// Before
for _, p := range items {
    p := p  // no longer needed
    go func() { use(p) }()
}

// After
for _, p := range items {
    go func() { use(p) }()
}
```

**any** - Replace `interface{}` with `any`:
```go
// Before
map[string]interface{}

// After
map[string]any
```

**mapsloop** - Use `maps.Copy`:
```go
// Before
for k, v := range src {
    dst[k] = v
}

// After
maps.Copy(dst, src)
```

**minmax** - Use `min`/`max` builtins:
```go
// Before
if count < 1 {
    count = 1
}

// After
count := max(w.Config.VMCount, 1)
```

### staticcheck - Static Analysis
**ST1023** - Omit redundant type in declaration:
```go
// Before
var name string = ""

// After
var name = ""
// or just: name := ""
```

**SA4000** - Identical expressions:
```go
// Fix the logic error - expressions like x - x are always 0
```

### gci / gofmt / golines - Formatting
These have suggested fixes - apply them directly using the base64-decoded `NewText`.

## Applying SuggestedFixes

When an issue has `SuggestedFixes`, decode and apply:

```go
import "encoding/base64"

// Decode NewText (if not null)
decoded, err := base64.StdEncoding.DecodeString(edit.NewText)

// Apply: replace bytes from Pos to End with decoded text
// If NewText is null, delete the range
```

## Workflow Commands

When invoked, I will:

1. **Parse Report**: Load and parse the JSON file
2. **Summarize**: Show issue counts by linter, file, and severity
3. **Ask Mode**: Interactive or batch?
4. **Fix Loop**:
   - Show issue details
   - Apply fix (suggested or generated)
   - Validate with tests if available
   - Continue or stop based on results

## Example Session

```
User: /golangci-lint tmp/golangci-lint-report.json

Claude: Parsed golangci-lint report. Found 55 issues:

By Linter:
- errcheck: 24 issues
- err113: 10 issues
- modernize: 9 issues
- gosec: 5 issues (2 high, 3 medium)
- gci: 2 issues
- staticcheck: 2 issues
- errname: 1 issue
- errorlint: 1 issue
- golines: 1 issue

How would you like to proceed?
1. Interactive mode (review each issue)
2. Batch mode (fix all issues from one linter)
3. Show issues by file
4. Show high-severity issues first
```
