// Datei kapselt die Ablagestruktur und Pfadauflosung fuer gespeicherte Dokumentdateien.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"bearstack/internal/fsutil"
)

var ErrFileTooLarge = errors.New("file exceeds configured upload limit")
var ErrInvalidFilename = errors.New("invalid filename")
var ErrUnsupportedFileType = errors.New("unsupported file type")

var errStoredPathEscapesRoot = errors.New("stored path escapes storage root")

const committedDocumentFilePerm os.FileMode = 0o640

type Store struct {
	root string
}

type Candidate struct {
	OriginalName string
	SafeName     string
	TempPath     string
	MIMEType     string
	SizeBytes    int64
	SHA256       string
}

func New(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, err
	}
	if _, err := ensureDirWithinRoot(abs, ".tmp", 0o750); err != nil {
		return nil, err
	}
	return &Store{root: abs}, nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) Receive(part *multipart.Part, maxBytes int64) (Candidate, error) {
	return s.ReceiveReader(part.FileName(), part, maxBytes)
}

func (s *Store) ReceiveReader(originalName string, r io.Reader, maxBytes int64) (Candidate, error) {
	original := filepath.Base(originalName)
	safe := SafeFilename(original)
	if safe == "" {
		return Candidate{}, ErrInvalidFilename
	}

	tmpDir, err := s.EnsureDir(".tmp")
	if err != nil {
		return Candidate{}, err
	}
	tmp, err := os.CreateTemp(tmpDir, "upload-*")
	if err != nil {
		return Candidate{}, err
	}
	tmpPath := tmp.Name()
	defer tmp.Close()

	hash := sha256.New()
	head := make([]byte, 0, 512)
	limited := &limitWriter{
		max: maxBytes,
		w: io.MultiWriter(tmp, hash, headWriter{
			write: func(p []byte) {
				if len(head) >= 512 {
					return
				}
				remain := 512 - len(head)
				if len(p) > remain {
					p = p[:remain]
				}
				head = append(head, p...)
			},
		}),
	}

	if _, err := io.Copy(limited, r); err != nil {
		_ = os.Remove(tmpPath)
		return Candidate{}, err
	}

	mimeType := detectMIME(head, safe)
	if !allowedMIME(mimeType, safe) {
		_ = os.Remove(tmpPath)
		return Candidate{}, fmt.Errorf("%w: %s", ErrUnsupportedFileType, mimeType)
	}

	return Candidate{
		OriginalName: original,
		SafeName:     safe,
		TempPath:     tmpPath,
		MIMEType:     mimeType,
		SizeBytes:    limited.n,
		SHA256:       hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func (s *Store) Commit(candidate Candidate, now time.Time) (string, error) {
	relDir := filepath.Join(fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", now.Month()))
	absDir, err := s.EnsureDir(relDir)
	if err != nil {
		return "", err
	}

	name := candidate.SafeName
	target := filepath.Join(absDir, name)
	for i := 1; ; i++ {
		if err := commitTempFile(candidate.TempPath, target); err == nil {
			return filepath.ToSlash(filepath.Join(relDir, name)), nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		ext := filepath.Ext(candidate.SafeName)
		base := strings.TrimSuffix(candidate.SafeName, ext)
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
		target = filepath.Join(absDir, name)
	}
}

func (s *Store) RemoveTemp(candidate Candidate) {
	if candidate.TempPath != "" {
		_ = os.Remove(candidate.TempPath)
	}
}

func (s *Store) Resolve(rel string) (string, error) {
	_, abs, err := fsutil.ResolveWithinRoot(s.root, rel, false, errStoredPathEscapesRoot)
	return abs, err
}

func (s *Store) EnsureDir(rel string) (string, error) {
	return ensureDirWithinRoot(s.root, rel, 0o750)
}

func (s *Store) Delete(rel string) error {
	path, err := s.Resolve(rel)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func SafeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\':
			return -1
		case r == ':' || r == '*':
			return '-'
		case r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			return '-'
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, ". ")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if len(name) > 180 {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if len(ext) > 30 {
			ext = ""
		}
		maxBase := 180 - len(ext)
		if maxBase < 1 {
			maxBase = 1
		}
		base = truncateUTF8Bytes(base, maxBase)
		name = base + ext
	}
	return name
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := 0
	for end < len(value) {
		_, width := utf8.DecodeRuneInString(value[end:])
		if width <= 0 {
			break
		}
		next := end + width
		if next > maxBytes {
			break
		}
		end = next
	}
	if end == 0 {
		return ""
	}
	return value[:end]
}

func ensureDirWithinRoot(root, rel string, perm os.FileMode) (string, error) {
	if root == "" {
		return "", errors.New("storage root is not configured")
	}
	return fsutil.EnsureDirWithinRoot(root, rel, perm, errStoredPathEscapesRoot)
}

func detectMIME(head []byte, name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".pdf" && len(head) >= 4 && string(head[:4]) == "%PDF" {
		return "application/pdf"
	}
	switch ext {
	case ".txt":
		if isLikelyText(head) {
			return "text/plain; charset=utf-8"
		}
	case ".md":
		if isLikelyText(head) {
			return "text/markdown; charset=utf-8"
		}
	case ".rtf":
		if len(head) >= 5 && strings.HasPrefix(string(head), `{\rtf`) {
			return "application/rtf"
		}
	case ".doc":
		if hasCompoundFileSignature(head) {
			return "application/msword"
		}
	case ".docx":
		if hasZipSignature(head) {
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		}
	case ".pages":
		if hasZipSignature(head) {
			return "application/vnd.apple.pages"
		}
	}
	mimeType := http.DetectContentType(head)
	if mimeType == "application/octet-stream" && ext == ".pdf" {
		return "application/pdf"
	}
	return mimeType
}

func allowedMIME(mimeType, name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".pdf":
		return mimeType == "application/pdf"
	case ".png":
		return mimeType == "image/png"
	case ".jpg", ".jpeg":
		return mimeType == "image/jpeg"
	case ".gif":
		return mimeType == "image/gif"
	case ".webp":
		return mimeType == "image/webp"
	case ".txt":
		return mimeBase(mimeType) == "text/plain"
	case ".md":
		return mimeBase(mimeType) == "text/markdown" || mimeBase(mimeType) == "text/plain"
	case ".rtf":
		return mimeBase(mimeType) == "application/rtf" || mimeBase(mimeType) == "text/rtf"
	case ".doc":
		return mimeBase(mimeType) == "application/msword"
	case ".docx":
		return mimeBase(mimeType) == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || mimeBase(mimeType) == "application/zip"
	case ".pages":
		return mimeBase(mimeType) == "application/vnd.apple.pages" || mimeBase(mimeType) == "application/zip"
	default:
		return false
	}
}

func mimeBase(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
}

func hasZipSignature(head []byte) bool {
	return len(head) >= 4 && head[0] == 'P' && head[1] == 'K' && (head[2] == 3 || head[2] == 5 || head[2] == 7) && (head[3] == 4 || head[3] == 6 || head[3] == 8)
}

func hasCompoundFileSignature(head []byte) bool {
	signature := []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}
	if len(head) < len(signature) {
		return false
	}
	for i := range signature {
		if head[i] != signature[i] {
			return false
		}
	}
	return true
}

func isLikelyText(head []byte) bool {
	if len(head) == 0 {
		return true
	}
	if !utf8.Valid(head) {
		return false
	}
	for _, r := range string(head) {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func commitTempFile(tempPath, target string) error {
	if err := os.Link(tempPath, target); err == nil {
		if err := setCommittedFilePermissions(target); err != nil {
			_ = os.Remove(target)
			return err
		}
		_ = os.Remove(tempPath)
		return nil
	} else if errors.Is(err, os.ErrExist) {
		return err
	}

	return copyTempFileNoReplace(tempPath, target)
}

func copyTempFileNoReplace(tempPath, target string) error {
	source, err := os.Open(tempPath)
	if err != nil {
		return err
	}
	defer source.Close()

	targetFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	cleanupTarget := true
	defer func() {
		if cleanupTarget {
			_ = os.Remove(target)
		}
	}()

	if _, err := io.Copy(targetFile, source); err != nil {
		_ = targetFile.Close()
		return err
	}
	if err := targetFile.Close(); err != nil {
		return err
	}
	if err := setCommittedFilePermissions(target); err != nil {
		return err
	}
	cleanupTarget = false
	_ = os.Remove(tempPath)
	return nil
}

func setCommittedFilePermissions(path string) error {
	return os.Chmod(path, committedDocumentFilePerm)
}

type limitWriter struct {
	max int64
	n   int64
	w   io.Writer
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.max > 0 && w.n+int64(len(p)) > w.max {
		allowed := w.max - w.n
		if allowed > 0 {
			_, _ = w.w.Write(p[:allowed])
			w.n += allowed
			return int(allowed), ErrFileTooLarge
		}
		return 0, ErrFileTooLarge
	}
	n, err := w.w.Write(p)
	w.n += int64(n)
	return n, err
}

type headWriter struct {
	write func([]byte)
}

func (w headWriter) Write(p []byte) (int, error) {
	w.write(p)
	return len(p), nil
}
