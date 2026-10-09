// Package tokens estimates LLM token counts.
package tokens

// Estimate approximates tokens as one per four bytes, the common rule of thumb
// for English text and code. Good enough for relative savings; not billing.
func Estimate(s string) int {
	return (len(s) + 3) / 4
}
