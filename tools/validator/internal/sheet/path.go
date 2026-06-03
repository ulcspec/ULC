package sheet

import (
	"fmt"
	"strings"
)

// setPath writes value into root at the dotted path (for example
// "product_family.manufacturer.slug"), creating intermediate maps as it
// descends. It returns an error rather than panicking when an intermediate
// segment already holds a non-map value, so malformed column specs surface as
// wrapped errors instead of runtime panics.
//
// Array-aware path handling (segments like "source_files[0]") is intentionally
// omitted: the records sheet carries no arrays, and the array-valued blocks
// (source_files, attestations, shared_attestations, covered_axes values,
// declared_by_cct, declared_by_length) are assembled directly from their own
// sheets and assigned as whole slices. This keeps the setter small and total.
func setPath(root map[string]any, path string, value any) error {
	segs := strings.Split(path, ".")
	if len(segs) == 0 || segs[0] == "" {
		return fmt.Errorf("empty dotted path")
	}
	node := root
	for i, seg := range segs {
		if seg == "" {
			return fmt.Errorf("empty segment in path %q", path)
		}
		if i == len(segs)-1 {
			node[seg] = value
			return nil
		}
		child, ok := node[seg]
		if !ok {
			next := map[string]any{}
			node[seg] = next
			node = next
			continue
		}
		nextMap, ok := child.(map[string]any)
		if !ok {
			return fmt.Errorf("path %q: segment %q is already a non-object value", path, seg)
		}
		node = nextMap
	}
	return nil
}

// getPath reads the value at a dotted path and whether it was present. It never
// panics on a non-map intermediate; it simply reports absent.
func getPath(root map[string]any, path string) (any, bool) {
	segs := strings.Split(path, ".")
	node := any(root)
	for i, seg := range segs {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[seg]
		if !ok {
			return nil, false
		}
		if i == len(segs)-1 {
			return v, true
		}
		node = v
	}
	return nil, false
}
