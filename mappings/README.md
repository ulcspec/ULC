# Mappings

This directory documents how ULC relates to adjacent standards in the lighting data ecosystem. ULC is designed to cooperate with, not replace, existing work.

## Files

- `gldf-crosswalk.md` is a field-level mapping between ULC records and GLDF (Global Lighting Data Format), with an embedded table for field-to-field correspondence.
- `etim-crosswalk.md` maps ULC fields to ETIM (ElectroTechnical Information Model) feature identifiers where applicable.
- `photometric-source-parsing.md` provides guidance for extracting ULC field values from IES LM-63 and EULUMDAT (LDT) photometric files.

## How ULC relates to these standards

- **GLDF** is the primary interchange container for the DIALux and RELUX planning ecosystems. GLDF and ULC address different problems: GLDF is an XML-based container optimized for photometric planning software, while ULC is a lightweight JSON specification optimized for structured datasheet data and AI consumption.
- **ETIM** provides a widely adopted classification vocabulary for product attributes in electrotechnical wholesale. ULC fields reference ETIM feature identifiers where applicable, supporting downstream compatibility with distributor data systems.
- **IES LM-63** and **EULUMDAT (LDT)** are the established photometric data formats. Their content feeds into ULC records; the files themselves are referenced, not replaced.

ULC does not redistribute the text of any paid or restricted standards. It references them by identifier.
