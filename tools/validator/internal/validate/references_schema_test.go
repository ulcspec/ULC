package validate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	embedded "github.com/ulcspec/ULC/schema"
)

// This test checks that the verification registry COVERS every file-reference
// site the schema mounts. tools/schema-drift-guard.py checks that pointers
// RESOLVE across the two schema files. Neither subsumes the other.

var applicatorKeywords = map[string]bool{
	"properties": true, "patternProperties": true, "items": true, "contains": true,
	"prefixItems": true, "additionalProperties": true, "propertyNames": true,
	"oneOf": true, "anyOf": true, "allOf": true, "if": true, "then": true,
	"else": true, "not": true,
}

var assertionKeywords = map[string]bool{
	"$schema": true, "$id": true, "$comment": true, "$defs": true, "definitions": true,
	"type": true, "title": true, "description": true, "examples": true, "default": true,
	"readOnly": true, "writeOnly": true, "deprecated": true, "format": true,
	"enum": true, "const": true, "required": true, "pattern": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true,
	"multipleOf": true, "minLength": true, "maxLength": true,
	"minItems": true, "maxItems": true, "uniqueItems": true,
	"minProperties": true, "maxProperties": true, "minContains": true, "maxContains": true,
	"dependentRequired": true,
}

type schemaWalk struct {
	defs   map[string]any
	target string
	found  map[string]bool
	faults []string
}

func (w *schemaWalk) walk(node any, ptr string, stack []string) {
	switch n := node.(type) {
	case []any:
		for _, v := range n {
			w.walk(v, ptr, stack)
		}
	case map[string]any:
		if raw, ok := n["$ref"]; ok {
			ref, _ := raw.(string)
			name, local := strings.CutPrefix(ref, "#/$defs/")
			if !local {
				if !strings.Contains(ref, ".schema.json#/$defs/") {
					w.faults = append(w.faults, fmt.Sprintf("unrecognized $ref form %q at %s", ref, ptr))
				}
				return // external file ref: leaf (taxonomy.schema.json carries no $ref)
			}
			if name == w.target {
				w.found[ptr] = true
				return
			}
			for _, s := range stack {
				if s == name {
					w.faults = append(w.faults, fmt.Sprintf("$defs cycle through %q at %s", name, ptr))
					return
				}
			}
			sub, ok := w.defs[name].(map[string]any)
			if !ok {
				w.faults = append(w.faults, fmt.Sprintf("dangling $ref %q at %s", ref, ptr))
				return
			}
			w.walk(sub, ptr, append(stack, name))
			return
		}
		for k, v := range n {
			switch {
			case k == "properties":
				kids, _ := v.(map[string]any)
				for pk, pv := range kids {
					w.walk(pv, ptr+"/"+pk, stack)
				}
			case k == "patternProperties":
				kids, _ := v.(map[string]any)
				for _, pv := range kids {
					w.walk(pv, ptr+"/<k>", stack)
				}
			case k == "items" || k == "contains" || k == "prefixItems":
				w.walk(v, ptr+"/<i>", stack)
			case k == "additionalProperties" || k == "propertyNames":
				if _, isSchema := v.(map[string]any); isSchema {
					w.walk(v, ptr+"/<k>", stack)
				}
			case applicatorKeywords[k]:
				w.walk(v, ptr, stack)
			case assertionKeywords[k]:
			default:
				w.faults = append(w.faults, fmt.Sprintf(
					"unrecognized schema keyword %q at %s: classify it as applicator or assertion in references_schema_test.go before merging", k, ptr))
			}
		}
	}
}

func schemaFamilies(t *testing.T, doc map[string]any, target string) []string {
	t.Helper()
	defs, _ := doc["$defs"].(map[string]any)
	w := &schemaWalk{defs: defs, target: target, found: map[string]bool{}}
	w.walk(doc, "", nil)
	for _, f := range w.faults {
		t.Errorf("schema walk: %s", f)
	}
	out := make([]string, 0, len(w.found))
	for p := range w.found {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func TestFileReferenceRegistryCoversSchema(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(embedded.ULCSchemaJSON, &doc); err != nil {
		t.Fatalf("parse ulc.schema.json: %v", err)
	}

	declared := map[string]bool{}
	for _, s := range fileReferenceRegistry {
		if s.policy == policyUnset {
			t.Errorf("registry row %q has no policy: every file-reference site must declare default or evidence-only verification", s.family)
		}
		if declared[s.family] {
			t.Errorf("registry declares %q twice", s.family)
		}
		declared[s.family] = true
	}

	actual := schemaFamilies(t, doc, "FileReference")
	t.Logf("schema-derived FileReference families: %v", actual)
	for _, fam := range actual {
		if !declared[fam] {
			t.Errorf("schema mounts a file reference at %q that the verification registry does not name. "+
				"Add a registry row with an explicit policy (default or evidence-only) and state the resulting "+
				"default-output change in the release notes.", fam)
		}
		delete(declared, fam)
	}
	for fam := range declared {
		t.Errorf("registry names %q but the schema no longer mounts a file reference there; remove the row", fam)
	}
}
