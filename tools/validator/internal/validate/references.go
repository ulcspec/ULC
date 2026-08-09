package validate

import (
	"strings"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// VerifyOptions selects which file-reference sites the walk visits.
type VerifyOptions struct {
	// Evidence turns on byte-verification of the attestation evidence
	// documents (`source_document_ref`). It is off by default because a
	// manufacturer may publish the reference while withholding the document,
	// and a default run must not report every withheld document forever.
	Evidence bool
}

// refPolicy states when a file-reference site is verified. The zero value is
// deliberately invalid: TestFileReferenceRegistryCoversSchema fails on it, so
// a site added in a later release must declare a policy rather than inherit
// one by accident.
type refPolicy int

const (
	policyUnset refPolicy = iota
	policyDefault
	policyEvidence
)

// enabled reports whether a site runs under these options. policyUnset never
// runs; the drift guard fails the build before such a site can ship.
func (p refPolicy) enabled(opts VerifyOptions) bool {
	switch p {
	case policyDefault:
		return true
	case policyEvidence:
		return opts.Evidence
	}
	return false
}

// refSite is one place the schema mounts a FileReference. `family` is the
// record-pointer pattern for the site, with `<i>` standing in for every array
// hop; the drift guard derives the same set from the schema's own applicator
// graph and fails when the two disagree. `walk` visits the site in a concrete
// record and reports the runtime pointer, with `<i>` replaced by the raw array
// index.
type refSite struct {
	family string
	policy refPolicy
	walk   func(recordDir string, record map[string]any, report *findings.Report)
}

// fileReferenceRegistry names every FileReference site in the schema and the
// policy that governs it. Adding a reference field to the schema without
// adding a row here fails TestFileReferenceRegistryCoversSchema.
var fileReferenceRegistry = []refSite{
	arraySite(policyDefault, []string{"source_files"}, "reference"),
	objectSite(policyDefault, []string{"product_family", "cutsheet"}),
	objectSite(policyDefault, []string{"emergency", "photometry_reference"}),
	arraySite(policyEvidence, []string{"attestations"}, "source_document_ref"),
	arraySite(policyEvidence, []string{"product_family", "shared_attestations"}, "source_document_ref"),
}

// VerifyFileReferences checks every file reference the record declares against
// the bytes on disk, at every site the schema mounts one. A file whose local
// path is not resolvable from recordDir emits INFO (the reader may not hold
// the source files); a file whose SHA-256 does not match its declared hash
// emits ERROR.
//
// recordDir is the directory the record lives in, used to resolve relative
// `filename` entries.
//
// Every site is verified independently, so a file referenced at two sites is
// hashed once per site and reports at each pointer: each site carries its own
// declared hash, and each one's verification status has to be visible where it
// is declared.
//
// Non-map nodes at any site are skipped silently. Schema validation owns
// complaints about shape.
func VerifyFileReferences(recordDir string, record map[string]any, opts VerifyOptions, report *findings.Report) {
	for _, site := range fileReferenceRegistry {
		if !site.policy.enabled(opts) {
			continue
		}
		site.walk(recordDir, record, report)
	}
}

// objectSite describes a reference mounted at a fixed chain of object keys,
// such as product_family.cutsheet. The family and the runtime pointer are the
// same string, built from the same keys the walk follows.
func objectSite(policy refPolicy, keys []string) refSite {
	pointer := "/" + strings.Join(keys, "/")
	return refSite{
		family: pointer,
		policy: policy,
		walk: func(recordDir string, record map[string]any, report *findings.Report) {
			ref, ok := objectAt(record, keys)
			if !ok {
				return
			}
			verifyOne(recordDir, ref, pointer, report)
		},
	}
}

// arraySite describes a reference mounted under `field` on every element of
// the array at `keys`, such as attestations[].source_document_ref. Runtime
// pointers carry the RAW array index, so an entry skipped for shape does not
// shift the pointers of the entries after it.
func arraySite(policy refPolicy, keys []string, field string) refSite {
	root := strings.Join(keys, "/")
	return refSite{
		family: "/" + root + "/<i>/" + field,
		policy: policy,
		walk: func(recordDir string, record map[string]any, report *findings.Report) {
			for i, entry := range arrayAt(record, keys) {
				m, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				ref, ok := m[field].(map[string]any)
				if !ok {
					continue
				}
				verifyOne(recordDir, ref, jsonPath(root, i)+"/"+field, report)
			}
		},
	}
}

// objectAt follows a chain of object keys from the record root.
func objectAt(record map[string]any, keys []string) (map[string]any, bool) {
	node := record
	for _, k := range keys {
		next, ok := node[k].(map[string]any)
		if !ok {
			return nil, false
		}
		node = next
	}
	return node, true
}

// arrayAt follows a chain of object keys from the record root to an array.
func arrayAt(record map[string]any, keys []string) []any {
	parent, ok := objectAt(record, keys[:len(keys)-1])
	if !ok {
		return nil
	}
	arr, _ := parent[keys[len(keys)-1]].([]any)
	return arr
}
