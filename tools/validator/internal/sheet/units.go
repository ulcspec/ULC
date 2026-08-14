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

// The four per-family conversion factors, shared by the outward
// (companionValue) and entry-side (siValue) directions so each family is
// governed by exactly one constant. Temperature converts by formula, not
// factor. Documented publicly in docs/conversion-policy.md, which ships in
// every release archive; the two surfaces are pinned against each other by
// test.
const (
	mmPerInch        = 25.4
	lbPerKg          = 2.2046226
	ft2PerM2         = 10.7639
	lbPerFtPerKgPerM = 0.671969
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
		return roundTo(si/mmPerInch, 4)
	case dualMass:
		return roundTo(si*lbPerKg, 1)
	case dualTemperature:
		return si*9/5 + 32
	case dualArea:
		return roundTo(si*ft2PerM2, 4)
	case dualMassPerLength:
		return roundTo(si*lbPerFtPerKgPerM, 4)
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

// siValue computes the SI leaf from an authored Imperial (or Fahrenheit)
// companion, inverting the same per-family rule companionValue applies
// outward, so both directions of a family are governed by one rule. Every
// computed SI leaf is rounded to <= 4 decimal places, the treatment the
// Pattern D length axis already applies (see linear.go): the rounding removes
// binary floating-point artifacts (96 in * 25.4 evaluates to
// 2438.3999999999996), it does not coarsen authored precision.
//
//	mm       = in * 25.4            rounded to <= 4 dp
//	kg       = lb / 2.2046226       rounded to <= 4 dp
//	c        = (f - 32) * 5/9       rounded to <= 4 dp
//	m2       = ft2 / 10.7639        rounded to <= 4 dp
//	kg_per_m = lb_per_ft / 0.671969 rounded to <= 4 dp
func (k dualUnitKind) siValue(companion float64) float64 {
	switch k {
	case dualLength:
		return roundTo(companion*mmPerInch, 4)
	case dualMass:
		return roundTo(companion/lbPerKg, 4)
	case dualTemperature:
		return roundTo((companion-32)*5/9, 4)
	case dualArea:
		return roundTo(companion/ft2PerM2, 4)
	case dualMassPerLength:
		return roundTo(companion/lbPerFtPerKgPerM, 4)
	default:
		return 0
	}
}

// buildDualUnitFromImperial assembles a DualUnit object from an authored
// Imperial (or Fahrenheit) value: the authored leaf is written verbatim so the
// published number appears in the record exactly as printed, the SI leaf is
// computed via siValue, and value_type and provenance follow the same rules as
// buildDualUnit. Only the computed side is ever rounded.
func buildDualUnitFromImperial(k dualUnitKind, companion float64, valueType string, provenance map[string]any) map[string]any {
	siLeaf, companionLeaf := k.leaves()
	obj := map[string]any{
		siLeaf:        numberLeaf(k.siValue(companion)),
		companionLeaf: numberLeaf(companion),
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

// maxInt64AsFloat is 2^63, the first float64 strictly above math.MaxInt64.
// math.MaxInt64 (2^63-1) is not representable in float64 and rounds up to this
// value, so the integer-range guard must be a strict `< maxInt64AsFloat`: a
// `<= math.MaxInt64` test would admit v == 2^63 and int64(2^63) wraps to
// math.MinInt64. math.MinInt64 (-2^63) IS exactly representable, so the lower
// bound stays inclusive.
const maxInt64AsFloat = 9223372036854775808.0

// numberLeaf preserves integer-ness so an authored "113" emits 113, not 113.0,
// matching how the index builder and Python json.dumps render whole numbers.
func numberLeaf(v float64) any {
	if v == math.Trunc(v) && !math.IsInf(v, 0) && v >= math.MinInt64 && v < maxInt64AsFloat {
		return int64(v)
	}
	return v
}
