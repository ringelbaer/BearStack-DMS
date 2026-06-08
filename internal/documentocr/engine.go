// Datei kapselt die lokale OCR-Ausfuehrung mit tesseract und poppler.
package documentocr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	Timeout                 = 20 * time.Minute
	pdfBulkFallbackMaxPages = 50
)

type ProgressFunc func(currentPage, totalPages int, message string) error

type LocalEngine struct{}

func (LocalEngine) CheckAvailable(mimeType string) error {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return errors.New("tesseract ist lokal nicht installiert oder nicht im PATH")
	}
	if mimeType == "application/pdf" {
		if _, err := exec.LookPath("pdftoppm"); err != nil {
			return errors.New("pdftoppm ist lokal nicht installiert oder nicht im PATH")
		}
		return nil
	}
	if strings.HasPrefix(mimeType, "image/") {
		return nil
	}
	return errors.New("OCR ist nur für PDF- und Bilddateien verfügbar")
}

func (e LocalEngine) Document(ctx context.Context, source, mimeType, lang string, progress ProgressFunc) (string, error) {
	if err := e.CheckAvailable(mimeType); err != nil {
		return "", err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	switch {
	case mimeType == "application/pdf":
		return PDF(timeoutCtx, source, lang, progress)
	case strings.HasPrefix(mimeType, "image/"):
		if progress != nil {
			if err := progress(0, 1, "Bild wird gelesen."); err != nil {
				return "", err
			}
		}
		text, err := Image(timeoutCtx, source, lang)
		if err != nil {
			return "", err
		}
		if progress != nil {
			if err := progress(1, 1, "Bild wurde gelesen. Textinhalt wird gespeichert."); err != nil {
				return "", err
			}
		}
		return text, nil
	default:
		return "", errors.New("OCR ist nur für PDF- und Bilddateien verfügbar")
	}
}

func PDF(ctx context.Context, source, lang string, progress ProgressFunc) (string, error) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return "", errors.New("pdftoppm ist lokal nicht installiert oder nicht im PATH")
	}

	tempDir, err := os.MkdirTemp("", "bearstack-ocr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	if totalPages, err := pdfPageCount(ctx, source); err == nil && totalPages > 0 {
		return pdfByPage(ctx, source, lang, tempDir, totalPages, progress)
	}
	if ctx.Err() != nil {
		return "", fmt.Errorf("pdfinfo: %w", ctx.Err())
	}
	return pdfBulk(ctx, source, lang, tempDir, progress)
}

func pdfByPage(ctx context.Context, source, lang, tempDir string, totalPages int, progress ProgressFunc) (string, error) {
	if progress != nil {
		if err := progress(0, totalPages, fmt.Sprintf("PDF enthält %d Seiten. OCR beginnt.", totalPages)); err != nil {
			return "", err
		}
	}

	var collected textCollector
	for pageNumber := 1; pageNumber <= totalPages; pageNumber++ {
		if progress != nil {
			if err := progress(pageNumber-1, totalPages, fmt.Sprintf("Seite %d von %d wird vorbereitet.", pageNumber, totalPages)); err != nil {
				return "", err
			}
		}
		pagePath, err := renderPDFPage(ctx, source, tempDir, pageNumber)
		if err != nil {
			return "", err
		}
		if progress != nil {
			if err := progress(pageNumber-1, totalPages, fmt.Sprintf("Seite %d von %d wird gelesen.", pageNumber, totalPages)); err != nil {
				return "", err
			}
		}
		pageText, err := Image(ctx, pagePath, lang)
		_ = os.Remove(pagePath)
		if err != nil {
			return "", err
		}
		collected.Append(pageText)
		if progress != nil {
			if err := progress(pageNumber, totalPages, fmt.Sprintf("%d von %d Seiten gelesen.", pageNumber, totalPages)); err != nil {
				return "", err
			}
		}
	}
	return collected.String(), nil
}

func pdfBulk(ctx context.Context, source, lang, tempDir string, progress ProgressFunc) (string, error) {
	if progress != nil {
		if err := progress(0, 0, pdfBulkProgressMessage()); err != nil {
			return "", err
		}
	}
	prefix := filepath.Join(tempDir, "page")
	cmd := exec.CommandContext(ctx, "pdftoppm", pdfBulkCommandArgs(source, prefix)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("pdftoppm: %w", ctx.Err())
		}
		return "", fmt.Errorf("pdftoppm: %w: %s", err, strings.TrimSpace(string(output)))
	}

	pages, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return "", err
	}
	sort.Slice(pages, func(i, j int) bool {
		return pageNumber(pages[i]) < pageNumber(pages[j])
	})
	if len(pages) == 0 {
		return "", errors.New("OCR konnte keine PDF-Seiten erzeugen")
	}
	if len(pages) > pdfBulkFallbackMaxPages {
		return "", fmt.Errorf("PDF-Seitenzahl konnte nicht bestimmt werden; der OCR-Fallback verarbeitet maximal %d Seiten", pdfBulkFallbackMaxPages)
	}
	if progress != nil {
		if err := progress(0, len(pages), fmt.Sprintf("%d PDF-Seiten vorbereitet. OCR beginnt.", len(pages))); err != nil {
			return "", err
		}
	}

	var collected textCollector
	for i, page := range pages {
		if progress != nil {
			if err := progress(i, len(pages), fmt.Sprintf("Seite %d von %d wird gelesen.", i+1, len(pages))); err != nil {
				return "", err
			}
		}
		text, err := Image(ctx, page, lang)
		if err != nil {
			return "", err
		}
		collected.Append(text)
		if progress != nil {
			if err := progress(i+1, len(pages), fmt.Sprintf("%d von %d Seiten gelesen.", i+1, len(pages))); err != nil {
				return "", err
			}
		}
	}
	return collected.String(), nil
}

func pdfBulkProgressMessage() string {
	return fmt.Sprintf("PDF-Seiten werden vorbereitet. Bei großen PDFs kann das mehrere Minuten dauern; nach 20 Minuten bricht BearStack ab. Ohne Seitenzählung verarbeitet BearStack maximal %d Seiten.", pdfBulkFallbackMaxPages)
}

func pdfBulkCommandArgs(source, prefix string) []string {
	return []string{"-f", "1", "-l", strconv.Itoa(pdfBulkFallbackMaxPages + 1), "-r", "300", "-png", source, prefix}
}

func pdfPageCount(ctx context.Context, source string) (int, error) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return 0, err
	}
	cmd := exec.CommandContext(ctx, "pdfinfo", source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("pdfinfo: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return parsePDFPageCount(string(output))
}

func parsePDFPageCount(output string) (int, error) {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(strings.ToLower(key)) != "pages" {
			continue
		}
		pages, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || pages <= 0 {
			return 0, errors.New("pdfinfo lieferte keine gültige Seitenzahl")
		}
		return pages, nil
	}
	return 0, errors.New("pdfinfo lieferte keine Seitenzahl")
}

func renderPDFPage(ctx context.Context, source, tempDir string, pageNumber int) (string, error) {
	prefix := filepath.Join(tempDir, fmt.Sprintf("page-%d", pageNumber))
	cmd := exec.CommandContext(ctx, "pdftoppm", "-f", strconv.Itoa(pageNumber), "-l", strconv.Itoa(pageNumber), "-singlefile", "-r", "300", "-png", source, prefix)
	if output, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("pdftoppm: %w", ctx.Err())
		}
		return "", fmt.Errorf("pdftoppm: %w: %s", err, strings.TrimSpace(string(output)))
	}
	pagePath := prefix + ".png"
	info, err := os.Stat(pagePath)
	if err != nil {
		return "", err
	}
	if info.Size() == 0 {
		return "", errors.New("pdftoppm erzeugte eine leere Seite")
	}
	return pagePath, nil
}

func Image(ctx context.Context, source, lang string) (string, error) {
	cmd := exec.CommandContext(ctx, "tesseract", source, "stdout", "-l", lang)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("tesseract: %w", ctx.Err())
		}
		message := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			message = strings.TrimSpace(string(exitErr.Stderr))
		}
		if message == "" {
			return "", fmt.Errorf("tesseract: %w", err)
		}
		return "", fmt.Errorf("tesseract: %w: %s", err, message)
	}
	return normalizeText(string(output)), nil
}

func pageNumber(path string) int {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	dash := strings.LastIndex(name, "-")
	if dash < 0 || dash == len(name)-1 {
		return 0
	}
	page, err := strconv.Atoi(name[dash+1:])
	if err != nil {
		return 0
	}
	return page
}

type textCollector struct {
	text strings.Builder
}

var lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\f", "\n")

func (c *textCollector) Append(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if c.text.Len() > 0 {
		c.text.WriteString("\n\n")
	}
	c.text.WriteString(value)
}

func (c *textCollector) String() string {
	return c.text.String()
}

func normalizeText(value string) string {
	value = lineBreaks.Replace(value)
	var cleaned strings.Builder
	blank := false
	wrote := false
	for {
		line, rest, found := strings.Cut(value, "\n")
		line = strings.TrimSpace(line)
		if line == "" {
			if wrote {
				blank = true
			}
		} else {
			if wrote {
				if blank {
					cleaned.WriteString("\n\n")
				} else {
					cleaned.WriteByte('\n')
				}
			}
			cleaned.WriteString(line)
			wrote = true
			blank = false
		}
		if !found {
			break
		}
		value = rest
	}
	return cleaned.String()
}
