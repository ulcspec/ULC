package sheet

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ReadCSVBundle reads a directory of `<sheet>.csv` files into a Workbook. The
// file stem (without the .csv extension) is the sheet name; the first row is
// the header (column names). Cells are trimmed of surrounding whitespace, and a
// blank cell is treated as absent (the key is omitted from the Row) so that
// "empty" is never confused with the literal value 0.
//
// Only the standard library encoding/csv is used, so the reader is offline and
// dependency-free. The native .xlsx reader in xlsx.go produces the same
// Workbook model from a single-file workbook.
func ReadCSVBundle(dir string) (Workbook, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read bundle dir %s: %w", dir, err)
	}
	// Sort entries so sheet discovery is deterministic regardless of the
	// filesystem's directory ordering.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	wb := Workbook{}
	for _, name := range names {
		sheetName := strings.TrimSuffix(name, filepath.Ext(name))
		rows, err := readCSVFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", sheetName, err)
		}
		wb[sheetName] = rows
	}
	return wb, nil
}

// readCSVFile parses one CSV file into a slice of Rows. The header row names the
// columns; each subsequent record becomes a Row. Trailing empty data rows
// (every cell blank) are skipped so a spreadsheet export's padding does not
// produce phantom records.
func readCSVFile(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate ragged rows; trailing blanks are common in exports
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return []Row{}, nil // empty file: a sheet with no header and no rows
		}
		return nil, fmt.Errorf("read header: %w", err)
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}

	rows := []Row{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read record: %w", err)
		}
		row := Row{}
		allBlank := true
		for i, col := range header {
			if col == "" || i >= len(rec) {
				continue
			}
			v := strings.TrimSpace(rec[i])
			if v == "" {
				continue // blank cell = absent
			}
			row[col] = v
			allBlank = false
		}
		if allBlank {
			continue // skip a fully blank padding row
		}
		rows = append(rows, row)
	}
	return rows, nil
}
