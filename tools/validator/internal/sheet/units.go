package sheet

import "math"

// dualUnitKind names a dual-unit family. The author supplies the SI/authoritative
// leaf; the converter computes the Imperial (or Fahrenheit) companion and writes
// both leaves plus the schema-required value_type and provenance.
type dualUnitKind int

const (
	dualLength        dualUnitKind = iota // mm -> in
	dualMass                              // kg -> lb
	dualTemperature                       // c  -> f
	dualArea                              // m2 -> ft2
	dualMassPerLength                     // kg_per_m -> lb_per_ft
)

// dualUnitLeaves returns the SI leaf key and the companion leaf key for a kind.
// These are the JSON property names the schema requires on each DualUnit object
// (for example "mm" and "in" for DualUnitLength).
func (k dualUnitKind) leaves() (si, companion string) {
	switch k {
	case dualLength:
		return "mm", "in"
	case dualMass:
		return "kg", "lb"
	case dualTemperature:
		return "c", "f"
	case dualArea:
		return "m2", "ft2"
	case dualMassPerLength:
		return "kg_per_m", "lb_per_ft"
	default:
		return "", ""
	}
}

// companionValue computes the Imperial/Fahrenheit companion from the SI value,
// applying the rounding rule DESIGN.md section 3.1 fixes for each family:
//
//	in        = mm / 25.4          rounded to <= 4 dp
//	lb        = kg * 2.2046226     rounded to 1 dp
//	f         = c * 9/5 + 32       (no extra rounding; mirrors the source data)
//	ft2       = m2 * 10.7639       rounded to 4 dp (area companion)
//	lb_per_ft = kg_per_m * 0.671969 rounded to 4 dp
func (k dualUnitKind) companionValue(si float64) float64 {
	switch k {
	case dualLength:
		return roundTo(si/25.4, 4)
	case dualMass:
		return roundTo(si*2.2046226, 1)
	case dualTemperature:
		return si*9/5 + 32
	case dualArea:
		return roundTo(si*10.7639, 4)
	case dualMassPerLength:
		return roundTo(si*0.671969, 4)
	default:
		return 0
	}
}

// buildDualUnit assembles a DualUnit object from the authored SI value: both
// leaves, the value_type, and the provenance block. provenance may be nil when
// the schema does not require it on this dual-unit family (it is required only
// on ProvenancedNumber; DualUnit carries it optionally), in which case the key
// is omitted.
func buildDualUnit(k dualUnitKind, si float64, valueType string, provenance map[string]any) map[string]any {
	siLeaf, companionLeaf := k.leaves()
	obj := map[string]any{
		siLeaf:        numberLeaf(si),
		companionLeaf: numberLeaf(k.companionValue(si)),
	}
	if valueType != "" {
		obj["value_type"] = valueType
	}
	if len(provenance) > 0 {
		obj["provenance"] = provenance
	}
	return obj
}

// roundTo rounds v to n decimal places using round-half-away-from-zero, the
// behavior callers expect for human-facing dimensional companions.
func roundTo(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

// numberLeaf preserves integer-ness so an authored "113" emits 113, not 113.0,
// matching how the index builder and Python json.dumps render whole numbers.
func numberLeaf(v float64) any {
	if v == math.Trunc(v) && !math.IsInf(v, 0) && v >= math.MinInt64 && v <= math.MaxInt64 {
		return int64(v)
	}
	return v
}
