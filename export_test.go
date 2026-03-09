// export_test.go — compiled only during tests.
// Re-exports unexported symbols from package main for use by the external
// test package (test/).
package main

// DeleteWordBefore wraps the unexported deleteWordBefore function.
var DeleteWordBefore = deleteWordBefore

// WrapText wraps the unexported wrapText function.
var WrapText = wrapText

// WordWrap wraps the unexported wordWrap function.
var WordWrap = wordWrap

// ShortPath wraps the unexported shortPath function.
var ShortPath = shortPath

// IsCd wraps the unexported isCd function.
var IsCd = isCd
