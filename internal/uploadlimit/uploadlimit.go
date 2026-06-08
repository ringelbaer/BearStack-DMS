// Package uploadlimit defines shared request and message size limits for upload paths.
package uploadlimit

const (
	// DefaultMaxBytes is the fallback envelope limit when no upload limit is configured.
	DefaultMaxBytes = 100 << 20

	// EnvelopeFactor allows multipart and mail envelope overhead around one file limit.
	EnvelopeFactor = 20
)

// EnvelopeLimit returns the maximum HTTP request or raw mail message size.
func EnvelopeLimit(maxFileBytes int64) int64 {
	if maxFileBytes < 1 {
		return DefaultMaxBytes
	}
	if maxFileBytes > (1<<62)/EnvelopeFactor {
		return maxFileBytes
	}
	return maxFileBytes * EnvelopeFactor
}
