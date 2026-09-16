package types

// Text classification, shared so the git and no-git paths cannot drift apart:
// both answer "is this a blob the terminal cannot render, or text that happens
// to carry a control byte?", and a reviewer should get the same answer either
// way.

// IsControlByte reports whether a byte is one the terminal acts on rather than
// draws. Tab, newline and carriage return are excluded: they are how text is
// written, not anomalies, and counting CR would make every line of a CRLF repo
// look suspect.
func IsControlByte(b byte) bool {
	switch b {
	case '\t', '\n', '\r':
		return false
	}
	return b < 0x20 || b == 0x7f
}

const (
	// binaryControlRatio is the share of control bytes above which a sample is
	// treated as binary. Presence alone is not the test: a source file can carry
	// a stray NUL — and when it does, that byte is usually the very thing under
	// review, so hiding the content hides the defect. Real binaries are nowhere
	// near this line (a PNG or ELF header runs 30-50%, UTF-16 text 50%), and a
	// line of code would need a control byte every ten characters to reach it.
	binaryControlRatio = 0.10

	// minBinarySample floors the denominator, so a ratio taken over a couple of
	// short lines cannot decide the question: a two-line hunk carrying one NUL is
	// 10% of its own bytes and 0.4% of this, and only the second number means
	// anything. In effect nothing is called binary under ~26 control bytes.
	minBinarySample = 256
)

// LooksBinary judges a sample from its control-byte count and size.
//
// It is deliberately biased toward calling things text, because the two mistakes
// are not equal: text wrongly called binary hides the diff completely, while
// binary wrongly called text costs a screenful of glyphs the reviewer pages past.
func LooksBinary(control, total int) bool {
	if total < minBinarySample {
		total = minBinarySample
	}
	return float64(control)/float64(total) >= binaryControlRatio
}

// CountControlBytes returns how many bytes of b the terminal would act on.
func CountControlBytes(b []byte) int {
	n := 0
	for _, c := range b {
		if IsControlByte(c) {
			n++
		}
	}
	return n
}
