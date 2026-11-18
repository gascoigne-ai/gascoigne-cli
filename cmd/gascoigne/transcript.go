package main

import (
	"bufio"
	"io"
)

// transcriptLineMax caps the length of one transcript line; a variable so
// tests can lower it.
var transcriptLineMax = 64 << 20

// scanTranscript calls visit with each line of a JSONL transcript. The line is
// only valid during the call. A line over transcriptLineMax stops the scan
// with bufio.ErrTooLong: what was read so far must not pass for the whole file.
func scanTranscript(reader io.Reader, visit func(line []byte)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, transcriptLineMax)
	for scanner.Scan() {
		visit(scanner.Bytes())
	}
	return scanner.Err()
}
