package sheet

import (
	"fmt"
	"sort"
	"strings"
)

const (
	CompanionFamilyProvenance = "provenance"
	CompanionFamilyRevision   = "revision"
)

// CompanionHeaderSuffixes is the single declaration of the two legal
// companion families. Conversion and tests read this same table.
var CompanionHeaderSuffixes = map[string][]string{
	CompanionFamilyProvenance: {
		"__value_type",
		"__prov_source",
		"__prov_method",
		"__extension_method",
		"__base_attestation_ref",
		"__attestation_ref",
	},
	CompanionFamilyRevision: {
		"__revision_label",
		"__revision_date",
	},
}

func checkCompanionHeaders(wb Workbook) error {
	sheets := make([]string, 0, len(wb.Headers))
	for sheet := range wb.Headers {
		sheets = append(sheets, sheet)
	}
	sort.Strings(sheets)
	for _, sheet := range sheets {
		headers := wb.Header(sheet)
		families, orderedBases := companionBaseFamilies(sheet, headers)
		for _, header := range headers {
			separator := strings.Index(header, "__")
			if separator < 0 {
				continue
			}
			base := header[:separator]
			suffix := header[separator:]
			family, ok := families[base]
			if !ok {
				legal := strings.Join(orderedBases, ", ")
				if legal == "" {
					legal = "(none)"
				}
				return fmt.Errorf("sheet %q: companion column %q has no legal base on this sheet; legal bases in template order: %s", sheet, header, legal)
			}
			if !containsString(CompanionHeaderSuffixes[family], suffix) {
				return fmt.Errorf("sheet %q: companion column %q has unsupported suffix %q; base %q accepts: %s", sheet, header, suffix, base, strings.Join(CompanionHeaderSuffixes[family], ", "))
			}
		}
	}
	return nil
}

func companionBaseFamilies(sheet string, headers []string) (map[string]string, []string) {
	families := map[string]string{}
	if sheet == "records" {
		for _, column := range recordColumns {
			if column.Provenanced() {
				families[column.Header] = CompanionFamilyProvenance
			}
		}
		families["cutsheet_file"] = CompanionFamilyRevision
		families["warranty_conditions_file"] = CompanionFamilyRevision
	}
	for key := range supplementaryValueColumns {
		if key.sheet == sheet {
			families[key.field] = CompanionFamilyProvenance
		}
	}
	if sheet == "source_files" {
		families["filename"] = CompanionFamilyRevision
	}
	if sheet == "attestations" {
		families["source_document_file"] = CompanionFamilyRevision
	}

	ordered := make([]string, 0, len(families))
	seen := map[string]bool{}
	for _, header := range headers {
		if _, ok := families[header]; ok && !seen[header] {
			ordered = append(ordered, header)
			seen[header] = true
		}
	}
	residue := make([]string, 0, len(families)-len(ordered))
	for base := range families {
		if !seen[base] {
			residue = append(residue, base)
		}
	}
	sort.Strings(residue)
	ordered = append(ordered, residue...)
	return families, ordered
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
