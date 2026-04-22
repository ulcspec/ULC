package validate

import "os"

// osReadFile is an indirection to os.ReadFile so schema_test.go can keep its
// import list compact. Kept in a _test file so it is never linked into the
// production binary.
func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
