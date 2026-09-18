package mailarchive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"bearstack/internal/mailmime"
)

func TestBuildBoundsMIMENestingAndCleansUp(t *testing.T) {
	for _, depth := range []int{mailmime.MaxMultipartDepth, mailmime.MaxMultipartDepth + 1, 256} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			var b strings.Builder
			b.WriteString("From: original@example.com\r\nSubject: Nested mail\r\n")
			for i := 0; i < depth; i++ {
				fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=level%d\r\n\r\n--level%d\r\n", i, i)
			}
			b.WriteString("Content-Type: text/plain\r\n\r\nNormal body")
			for i := depth - 1; i >= 0; i-- {
				fmt.Fprintf(&b, "\r\n--level%d--\r\n", i)
			}
			tempDir := t.TempDir()
			result, err := Build(context.Background(), "nested.eml", strings.NewReader(b.String()), Options{MaxBytes: 1 << 20, TempDir: tempDir})
			if depth <= mailmime.MaxMultipartDepth {
				if err != nil {
					t.Fatal(err)
				}
				if info, err := os.Stat(result.Path); err != nil || info.Size() == 0 {
					t.Fatalf("archive = %v, error = %v", info, err)
				}
				result.Cleanup()
			} else if !errors.Is(err, mailmime.ErrMultipartTooDeep) {
				t.Fatalf("error = %v, want excessive MIME nesting", err)
			}
			entries, err := os.ReadDir(tempDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("remaining temporary entries = %v, error = %v", entries, err)
			}
		})
	}
}
