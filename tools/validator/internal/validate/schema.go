// Package validate runs structural, parity, and integrity checks on a ULC
// record and emits diagnostics through internal/findings.
//
// The schema check uses santhosh-tekuri/jsonschema/v6, which has the
// strongest JSON Schema Draft 2020-12 compliance among Go implementations.
// Both ulc.schema.json and taxonomy.schema.json are registered against the
// compiler so cross-file `$ref`s resolve.
package validate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	embedded "github.com/ulcspec/ULC/tools/validator/schema"
)

// Validator holds a compiled ULC schema ready for repeated Validate calls.
type Validator struct {
	schema *jsonschema.Schema
}

// NewValidator loads `ulc.schema.json` and `taxonomy.schema.json` from the
// given directory and returns a Validator. Schema files reference each other
// via relative URLs (`taxonomy.schema.json#/$defs/...`), which resolve
// against the base URL each file is registered under.
func NewValidator(schemaDir string) (*Validator, error) {
	ulcPath := filepath.Join(schemaDir, "ulc.schema.json")
	taxPath := filepath.Join(schemaDir, "taxonomy.schema.json")

	ulcDoc, err := loadJSON(ulcPath)
	if err != nil {
		return nil, err
	}
	taxDoc, err := loadJSON(taxPath)
	if err != nil {
		return nil, err
	}
	return compile(ulcDoc, taxDoc)
}

// NewValidatorEmbedded returns a Validator built from the schema files
// embedded into the binary at compile time. Used when the binary is running
// outside the source repository and no schema directory is available.
func NewValidatorEmbedded() (*Validator, error) {
	ulcDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(embedded.ULCSchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("parse embedded ulc.schema.json: %w", err)
	}
	taxDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(embedded.TaxonomySchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("parse embedded taxonomy.schema.json: %w", err)
	}
	return compile(ulcDoc, taxDoc)
}

// compile registers both schema documents under their canonical $id URLs and
// returns a Validator. Relative `$ref: "taxonomy.schema.json#/$defs/..."`
// refs in ulc.schema.json resolve against the base URL each file is
// registered under, so the compiler never needs a network fetch.
func compile(ulcDoc, taxDoc any) (*Validator, error) {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(ulcSchemaURL, ulcDoc); err != nil {
		return nil, fmt.Errorf("register ulc.schema.json: %w", err)
	}
	if err := compiler.AddResource(taxonomySchemaURL, taxDoc); err != nil {
		return nil, fmt.Errorf("register taxonomy.schema.json: %w", err)
	}
	sch, err := compiler.Compile(ulcSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile ulc.schema.json: %w", err)
	}
	return &Validator{schema: sch}, nil
}

// Canonical $id values of the two schema files. Must match the `$id` keyword
// in the schema JSON, or the compiler will refetch remotely.
const (
	ulcSchemaURL      = "https://ulcspec.org/schema/ulc.schema.json"
	taxonomySchemaURL = "https://ulcspec.org/schema/taxonomy.schema.json"
)

// Validate runs the compiled schema against a parsed record and appends any
// violations to the report as ERROR-level findings.
//
// `record` MUST be the json.Number-typed tree returned by jsonschema.UnmarshalJSON
// or json.NewDecoder().UseNumber(). Passing a tree whose numbers have already
// been coerced to int64 / float64 will cause certain type-keyword checks to
// misbehave.
func (v *Validator) Validate(record any, report *findings.Report) {
	err := v.schema.Validate(record)
	if err == nil {
		return
	}
	var verr *jsonschema.ValidationError
	if errors.As(err, &verr) {
		flattenValidationError(verr, report)
		return
	}
	// Non-validation error (e.g., compile failure propagated at validate time).
	report.AddError(findings.CodeSchemaViolation, "",
		fmt.Sprintf("schema validation could not complete: %v", err))
}

// flattenValidationError walks the nested ValidationError tree and emits one
// finding per leaf cause. The top-level error has a generic "doesn't validate"
// message; the leaves are where the actual per-field issues live.
func flattenValidationError(err *jsonschema.ValidationError, report *findings.Report) {
	// Depth-first walk, collecting only leaves (Causes is empty).
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			report.AddError(
				findings.CodeSchemaViolation,
				jsonPointerFromLocation(e.InstanceLocation),
				e.Error(),
			)
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(err)
}

// jsonPointerFromLocation converts jsonschema's instance-location slice into
// an RFC 6901 JSON Pointer. Empty location (root) returns "".
func jsonPointerFromLocation(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	escaped := make([]string, len(parts))
	for i, p := range parts {
		p = strings.ReplaceAll(p, "~", "~0")
		p = strings.ReplaceAll(p, "/", "~1")
		escaped[i] = p
	}
	return "/" + strings.Join(escaped, "/")
}

// loadJSON reads a file at path and returns the json.Number-typed parsed tree.
func loadJSON(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}

// FindSchemaDir locates the ULC schema directory using a priority chain:
//  1. explicit override (if non-empty)
//  2. ULC_SCHEMA_DIR environment variable
//  3. walk up from recordPath's dir looking for schema/ulc.schema.json
//  4. walk up from cwd looking for schema/ulc.schema.json
//
// Returns the directory path or an error describing the failed search.
func FindSchemaDir(override, recordPath string) (string, error) {
	if override != "" {
		if _, err := os.Stat(filepath.Join(override, "ulc.schema.json")); err != nil {
			return "", fmt.Errorf("--schema-dir %s: %w", override, err)
		}
		return override, nil
	}
	if env := os.Getenv("ULC_SCHEMA_DIR"); env != "" {
		if _, err := os.Stat(filepath.Join(env, "ulc.schema.json")); err != nil {
			return "", fmt.Errorf("ULC_SCHEMA_DIR=%s: %w", env, err)
		}
		return env, nil
	}
	searchRoots := []string{}
	if recordPath != "" {
		if abs, err := filepath.Abs(recordPath); err == nil {
			searchRoots = append(searchRoots, filepath.Dir(abs))
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		searchRoots = append(searchRoots, cwd)
	}
	for _, root := range searchRoots {
		if dir := walkUpForSchema(root); dir != "" {
			return dir, nil
		}
	}
	return "", fmt.Errorf("could not locate ULC schema directory. Pass --schema-dir PATH, set ULC_SCHEMA_DIR, or run from inside the ULC repository")
}

// walkUpForSchema walks up from start looking for schema/ulc.schema.json.
// Stops at filesystem root. Returns the schema directory or "".
func walkUpForSchema(start string) string {
	dir := start
	for {
		candidate := filepath.Join(dir, "schema", "ulc.schema.json")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(dir, "schema")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
