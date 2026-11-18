package dataoptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// The regexp lockNoise replaced, kept to pin the old behavior.
var oldLockNoise = lazyre.New(`^(checksum|hash|hashes|files|wheels|sdist|source|content-hash|resolution-markers|upload-time|size|url)$`)

func TestLockNoiseSameAsRegexp(t *testing.T) {
	examples := []string{"checksum", "hash", "hashes", "files", "wheels", "sdist", "source", "content-hash", "resolution-markers", "upload-time", "size", "url",
		"name", "version", "dependencies", "urls", "Size", "hash ", " hash", "", "sources"}
	sameAsOld(t, oldLockNoise.MatchString, func(key string) bool { return lockNoise[key] }, examples)
}
