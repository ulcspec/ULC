package sheet

import (
	"archive/zip"
	"fmt"
	"io"
)

// Worksheet XML amplifies into the Workbook model at a measured 12x retained
// and 18x peak in the worst legal shape (one cell per row), 3x to 5x in
// realistic shapes. The total cap is therefore the heap control:
// worst-case peak heap is about 18 x maxArchiveInflatedBytes.
const (
	maxArchiveEntries       = 1024
	maxPartInflatedBytes    = 16 << 20 // 16 MiB; about 25,000 realistic 12-column rows in one sheet
	maxArchiveInflatedBytes = 32 << 20 // 32 MiB across every opened part; about 600 MiB worst-case peak heap
	maxCompressionRatio     = 100
	ratioFloorBytes         = 1 << 20 // 1 MiB
)

// Limit values carried by ArchiveLimitError.
const (
	limitEntryCount       = "entry-count"
	limitPartBytes        = "part-bytes"
	limitTotalBytes       = "total-bytes"
	limitCompressionRatio = "compression-ratio"
)

// archiveLimitKind selects the message. Two limits are reachable from two
// places each: a part's central-directory claim is checked before any byte is
// inflated, and the bytes actually inflated are charged as they arrive.
type archiveLimitKind int

const (
	kindEntryCount archiveLimitKind = iota
	kindPartClaim
	kindTotalClaim
	kindRatioClaim
	kindPartDebit
	kindTotalDebit
)

// ArchiveLimitError reports a workbook archive that exceeds one of the
// reader's resource limits. It is errors.As-able through the wrapping the
// reader and encoding/xml apply on the way out.
type ArchiveLimitError struct {
	Path     string // the archive
	Part     string // the part at fault; empty for the entry-count pre-check
	Limit    string // which limit: entry-count, part-bytes, total-bytes, compression-ratio
	Max      int64  // the limit
	Observed int64  // the quantity that tripped it

	kind archiveLimitKind
}

func (e *ArchiveLimitError) Error() string {
	switch e.kind {
	case kindEntryCount:
		return fmt.Sprintf("archive has %d entries; the limit is %d", e.Observed, e.Max)
	case kindPartClaim:
		return fmt.Sprintf("part %q claims %d uncompressed bytes; the per-part limit is %d", e.Part, e.Observed, e.Max)
	case kindTotalClaim:
		return fmt.Sprintf("part %q claims %d bytes, which would take the archive past the %d-byte total limit", e.Part, e.Observed, e.Max)
	case kindRatioClaim:
		return fmt.Sprintf("part %q claims a compression ratio of %d:1; the limit is %d:1", e.Part, e.Observed, e.Max)
	case kindPartDebit:
		return fmt.Sprintf("part %q inflated past the %d-byte per-part limit after %d bytes", e.Part, e.Max, e.Observed)
	case kindTotalDebit:
		return fmt.Sprintf("part %q took the archive past the %d-byte total inflation limit after %d bytes", e.Part, e.Max, e.Observed)
	}
	return "archive limit exceeded"
}

// archiveBudget is the single allowance for inflated bytes across every part
// this reader opens. A part's central-directory claim is CHECKED against the
// allowance; the bytes actually inflated are CHARGED to it. Nothing is
// charged twice, and a part the reader never opens costs nothing.
type archiveBudget struct {
	path      string // archive path, for the error message
	remaining int64  // inflated bytes still allowed across all opened parts
}

func newArchiveBudget(path string) *archiveBudget {
	return &archiveBudget{path: path, remaining: maxArchiveInflatedBytes}
}

// checkEntryCount is the one pre-check, run once immediately after the archive
// opens. There is no central-directory sizing pass: a part's claim is read
// from the same header at open time, before a single byte is inflated, so
// deferring the size checks to openPart costs nothing and avoids charging
// parts the reader never opens (styles, theme, media) against a budget that
// exists to bound heap.
func checkEntryCount(path string, entries int) error {
	if entries > maxArchiveEntries {
		return &ArchiveLimitError{
			Path: path, Limit: limitEntryCount,
			Max: maxArchiveEntries, Observed: int64(entries),
			kind: kindEntryCount,
		}
	}
	return nil
}

// openPart is the only way this package opens an archive entry. The three
// gates read the central directory's claim and never mutate the budget; the
// counting reader that wraps the opened part is the only writer of it, so
// double counting is structurally impossible.
//
// Go's zip reader bounds delivered bytes by the claim (checksumReader.Read
// returns ErrFormat the instant delivered bytes exceed UncompressedSize64), so
// the claim gate is the per-part enforcement. The counting reader is defense
// in depth that keeps enforcement correct if that ever changes, and it is what
// charges the shared total.
func openPart(f *zip.File, b *archiveBudget) (io.ReadCloser, error) {
	claimed := int64(f.UncompressedSize64)
	comp := int64(f.CompressedSize64)

	if claimed > maxPartInflatedBytes {
		return nil, &ArchiveLimitError{
			Path: b.path, Part: f.Name, Limit: limitPartBytes,
			Max: maxPartInflatedBytes, Observed: claimed,
			kind: kindPartClaim,
		}
	}
	if claimed > b.remaining {
		return nil, &ArchiveLimitError{
			Path: b.path, Part: f.Name, Limit: limitTotalBytes,
			Max: maxArchiveInflatedBytes, Observed: claimed,
			kind: kindTotalClaim,
		}
	}
	if claimed >= ratioFloorBytes && comp > 0 && claimed/comp > maxCompressionRatio {
		return nil, &ArchiveLimitError{
			Path: b.path, Part: f.Name, Limit: limitCompressionRatio,
			Max: maxCompressionRatio, Observed: claimed / comp,
			kind: kindRatioClaim,
		}
	}

	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	return &countingPart{
		rc:            rc,
		path:          b.path,
		part:          f.Name,
		partRemaining: maxPartInflatedBytes,
		budget:        b,
	}, nil
}

// countingPart charges the bytes a part actually inflates against both the
// per-part allowance and the archive-wide budget, failing mid-stream when
// either goes negative. It is built fresh inside every openPart call:
// partRemaining must never be shared between parts, because a shared per-part
// counter would just be a second, tighter total cap.
type countingPart struct {
	rc            io.ReadCloser
	path          string
	part          string
	partRemaining int64
	budget        *archiveBudget
	read          int64
}

func (c *countingPart) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if n > 0 {
		c.read += int64(n)
		c.partRemaining -= int64(n)
		c.budget.remaining -= int64(n)
		if c.partRemaining < 0 {
			return n, &ArchiveLimitError{
				Path: c.path, Part: c.part, Limit: limitPartBytes,
				Max: maxPartInflatedBytes, Observed: c.read,
				kind: kindPartDebit,
			}
		}
		if c.budget.remaining < 0 {
			return n, &ArchiveLimitError{
				Path: c.path, Part: c.part, Limit: limitTotalBytes,
				Max: maxArchiveInflatedBytes, Observed: c.read,
				kind: kindTotalDebit,
			}
		}
	}
	return n, err
}

func (c *countingPart) Close() error { return c.rc.Close() }
