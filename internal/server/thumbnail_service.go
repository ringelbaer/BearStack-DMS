// Datei koordiniert Thumbnail-Erzeugung und Auslieferung fuer Dokumente und Fotos.
package server

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/documentconvert"
	"bearstack/internal/fsutil"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

type thumbnailService struct {
	repo  *repository.Repository
	store *storage.Store
	log   *slog.Logger
	jobs  chan struct{}
}

func (s *Server) thumbnailService() thumbnailRunner {
	if s.apps.documents.thumbnails != nil {
		return s.apps.documents.thumbnails
	}
	return newThumbnailService(s.repo, s.store, s.log, nil)
}

func newThumbnailService(repo *repository.Repository, store *storage.Store, log *slog.Logger, jobs chan struct{}) *thumbnailService {
	return &thumbnailService{
		repo:  repo,
		store: store,
		log:   log,
		jobs:  jobs,
	}
}

func (t thumbnailService) EnsureAll(ctx context.Context) error {
	total := 0
	for {
		docs, err := t.repo.ThumbnailCandidates(ctx, 10)
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			if total > 0 {
				logInfo(t.log, "thumbnail generation completed", "documents", total)
			}
			return nil
		}
		progress := 0
		for _, doc := range docs {
			if err := t.Ensure(ctx, doc); err != nil {
				logWarn(t.log, "thumbnail generation skipped document", "id", doc.ID, "error", err)
				continue
			}
			progress++
			total++
		}
		if progress == 0 {
			return nil
		}
	}
}

func (t thumbnailService) Ensure(ctx context.Context, doc document.Document) error {
	if doc.ID <= 0 {
		return nil
	}
	if doc.ThumbnailPath != "" {
		path, err := t.store.Resolve(doc.ThumbnailPath)
		if err == nil {
			if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
				return nil
			}
		}
	}
	if doc.MIMEType == "application/pdf" {
		return t.ensurePDFThumbnail(ctx, doc)
	}
	if isImageThumbnailMIME(doc.MIMEType) {
		return t.ensureImageThumbnail(ctx, doc)
	}
	if documentconvert.IsPreviewDocument(doc.OriginalName, doc.MIMEType) {
		return t.ensureOfficeThumbnail(ctx, doc)
	}
	return nil
}

func (t thumbnailService) ensurePDFThumbnail(ctx context.Context, doc document.Document) error {
	release, err := t.acquireJob(ctx)
	if err != nil {
		return err
	}
	defer release()

	thumbnailRel, thumbnailAbs, err := t.thumbnailTarget(doc.ID)
	if err != nil {
		return err
	}
	if fsutil.FileHasContent(thumbnailAbs) {
		return t.repo.UpdateThumbnailPath(ctx, doc.ID, thumbnailRel)
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return err
	}

	source, err := t.store.Resolve(doc.StoredPath)
	if err != nil {
		return err
	}
	if _, err := t.store.EnsureDir(filepath.ToSlash(filepath.Dir(thumbnailRel))); err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := writePDFThumbnail(timeoutCtx, source, thumbnailAbs); err != nil {
		return err
	}
	return t.repo.UpdateThumbnailPath(ctx, doc.ID, thumbnailRel)
}

func (t thumbnailService) ensureOfficeThumbnail(ctx context.Context, doc document.Document) error {
	release, err := t.acquireJob(ctx)
	if err != nil {
		return err
	}
	defer release()

	thumbnailRel, thumbnailAbs, err := t.thumbnailTarget(doc.ID)
	if err != nil {
		return err
	}
	if fsutil.FileHasContent(thumbnailAbs) {
		return t.repo.UpdateThumbnailPath(ctx, doc.ID, thumbnailRel)
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return err
	}
	if _, err := t.store.EnsureDir(filepath.ToSlash(filepath.Dir(thumbnailRel))); err != nil {
		return err
	}

	previewAbs, err := t.EnsureOfficePreview(ctx, doc)
	if err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := writePDFThumbnail(timeoutCtx, previewAbs, thumbnailAbs); err != nil {
		return err
	}
	return t.repo.UpdateThumbnailPath(ctx, doc.ID, thumbnailRel)
}

func writePDFThumbnail(ctx context.Context, source, target string) error {
	prefix := strings.TrimSuffix(target, filepath.Ext(target))
	cmd := exec.CommandContext(ctx, "pdftoppm", "-f", "1", "-l", "1", "-singlefile", "-jpeg", "-scale-to", "300", source, prefix)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pdftoppm: %w: %s", err, strings.TrimSpace(string(output)))
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return errors.New("leeres Vorschaubild erzeugt")
	}
	return nil
}

func (t thumbnailService) ensureImageThumbnail(ctx context.Context, doc document.Document) error {
	release, err := t.acquireJob(ctx)
	if err != nil {
		return err
	}
	defer release()

	thumbnailRel, thumbnailAbs, err := t.thumbnailTarget(doc.ID)
	if err != nil {
		return err
	}
	if fsutil.FileHasContent(thumbnailAbs) {
		return t.repo.UpdateThumbnailPath(ctx, doc.ID, thumbnailRel)
	}
	source, err := t.store.Resolve(doc.StoredPath)
	if err != nil {
		return err
	}
	if _, err := t.store.EnsureDir(filepath.ToSlash(filepath.Dir(thumbnailRel))); err != nil {
		return err
	}
	if err := writeDocumentImageThumbnail(source, thumbnailAbs, 300); err != nil {
		_ = os.Remove(thumbnailAbs)
		return err
	}
	return t.repo.UpdateThumbnailPath(ctx, doc.ID, thumbnailRel)
}

func (t thumbnailService) thumbnailTarget(docID int64) (string, string, error) {
	thumbnailRel := filepath.ToSlash(filepath.Join(".thumbnails", fmt.Sprintf("%d.jpg", docID)))
	thumbnailAbs, err := t.store.Resolve(thumbnailRel)
	if err != nil {
		return "", "", err
	}
	return thumbnailRel, thumbnailAbs, nil
}

func (t thumbnailService) acquireJob(ctx context.Context) (func(), error) {
	if t.jobs == nil {
		return func() {}, nil
	}
	select {
	case t.jobs <- struct{}{}:
		return func() { <-t.jobs }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func isImageThumbnailMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/png", "image/gif":
		return true
	default:
		return false
	}
}

func writeDocumentImageThumbnail(source, target string, size int) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}
	scaled := scaleDocumentThumbnailImage(img, size)
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".*.jpg")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()
	encodeErr := jpeg.Encode(tmp, scaled, &jpeg.Options{Quality: 84})
	closeErr := tmp.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !fsutil.FileHasContent(tmpName) {
		return errors.New("leeres Vorschaubild erzeugt")
	}
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	renamed = true
	return nil
}

func scaleDocumentThumbnailImage(src image.Image, maxSize int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	targetWidth, targetHeight := width, height
	if width > height && width > maxSize {
		targetWidth = maxSize
		targetHeight = height * maxSize / width
	} else if height >= width && height > maxSize {
		targetHeight = maxSize
		targetWidth = width * maxSize / height
	}
	if targetWidth < 1 {
		targetWidth = 1
	}
	if targetHeight < 1 {
		targetHeight = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	for y := 0; y < targetHeight; y++ {
		srcY := bounds.Min.Y + y*height/targetHeight
		for x := 0; x < targetWidth; x++ {
			srcX := bounds.Min.X + x*width/targetWidth
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}
