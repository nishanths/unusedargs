package main

import (
	"bytes"
	"testing"
)

func TestHandleFiles(t *testing.T) {
	var files = []string{
		"testdata/pkg1/pkg1.go",
		"testdata/pkg1/pkg1_test.go",
		"testdata/pkg1/ext_test.go",
		"testdata/pkg2/pkg2.go",
	}

	// TODO(nishanth): This is gross. Restructure main to fix.
	// Override globals.
	var buf bytes.Buffer
	output = &buf

	// Run the test.
	handleFiles(files)

	const want = `testdata/pkg1/pkg1.go:10:1: VarArgsUnused has unused param s
testdata/pkg1/pkg1.go:12:1: RegularArgsUnused has unused param y
testdata/pkg1/pkg1.go:14:1: NakedReturnUnused has unused param x
testdata/pkg1/pkg1.go:19:5: func has unused param x
testdata/pkg1/pkg1.go:21:6: func has unused param y
testdata/pkg1/pkg1.go:25:1: ScopeUnused has unused param n
testdata/pkg1/pkg1_test.go:3:1: bar has unused param x
testdata/pkg1/ext_test.go:3:1: bar has unused param x
testdata/pkg2/pkg2.go:3:1: qux has unused param x
`
	if want != buf.String() {
		t.Errorf("want: %s\ngot:  %s", want, buf.String())
	}
}
