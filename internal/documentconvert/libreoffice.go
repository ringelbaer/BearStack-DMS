// Package documentconvert kapselt optionale Dokumentkonvertierung ueber LibreOffice.
package documentconvert

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bearstack/internal/textmeta"
)

var ErrLibreOfficeUnavailable = errors.New("libreoffice soffice is not installed or not in PATH")

const libreOfficeTimeout = 2 * time.Minute

var libreOfficeJobs = make(chan struct{}, 1)

func IsLibreOfficeDocumentName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".rtf", ".doc", ".docx", ".pages":
		return true
	default:
		return false
	}
}

func IsLibreOfficeDocumentMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "text/rtf",
		"application/rtf",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.apple.pages":
		return true
	default:
		return false
	}
}

func IsLibreOfficeDocument(name, mimeType string) bool {
	return IsLibreOfficeDocumentName(name) || IsLibreOfficeDocumentMIME(mimeType)
}

func IsPlainTextDocumentName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".md":
		return true
	default:
		return false
	}
}

func IsPlainTextDocumentMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "text/plain", "text/markdown":
		return true
	default:
		return false
	}
}

func IsPlainTextDocument(name, mimeType string) bool {
	return IsPlainTextDocumentName(name) || IsPlainTextDocumentMIME(mimeType)
}

func IsPreviewDocument(name, mimeType string) bool {
	return IsPlainTextDocument(name, mimeType) || IsLibreOfficeDocument(name, mimeType)
}

func ConvertToPDF(ctx context.Context, source, target string) error {
	return convertToFile(ctx, source, target, "pdf")
}

func ExtractText(ctx context.Context, source string) (string, error) {
	tempDir, err := os.MkdirTemp("", "bearstack-libreoffice-text-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	if err := runLibreOfficeConvert(ctx, source, tempDir, "txt:Text"); err != nil {
		return "", err
	}
	output := filepath.Join(tempDir, strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))+".txt")
	file, err := os.Open(output)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return textmeta.ExtractPlainText(file, 10<<20), nil
}

func convertToFile(ctx context.Context, source, target, format string) error {
	tempDir, err := os.MkdirTemp(filepath.Dir(target), ".bearstack-libreoffice-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := runLibreOfficeConvert(ctx, source, tempDir, format); err != nil {
		return err
	}
	output := filepath.Join(tempDir, strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))+"."+format)
	info, err := os.Stat(output)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("libreoffice produced an empty %s file", format)
	}
	return replaceFile(output, target)
}

func runLibreOfficeConvert(ctx context.Context, source, outDir, format string) error {
	command, err := libreOfficeCommand()
	if err != nil {
		return err
	}
	profileDir, err := os.MkdirTemp("", "bearstack-libreoffice-profile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(profileDir)

	timeoutCtx, cancel := context.WithTimeout(ctx, libreOfficeTimeout)
	defer cancel()
	release, err := acquireLibreOffice(timeoutCtx)
	if err != nil {
		return err
	}
	defer release()

	profileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(profileDir)}).String()
	args := []string{
		"--headless",
		"--nologo",
		"--nofirststartwizard",
		"--nodefault",
		"--nolockcheck",
		"--norestore",
		"-env:UserInstallation=" + profileURL,
		"--convert-to", format,
		"--outdir", outDir,
		source,
	}
	cmd := exec.CommandContext(timeoutCtx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if timeoutCtx.Err() != nil {
			return fmt.Errorf("libreoffice: %w", timeoutCtx.Err())
		}
		return fmt.Errorf("libreoffice: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func acquireLibreOffice(ctx context.Context) (func(), error) {
	select {
	case libreOfficeJobs <- struct{}{}:
		return func() { <-libreOfficeJobs }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func libreOfficeCommand() (string, error) {
	if path, err := exec.LookPath("soffice"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("libreoffice"); err == nil {
		return path, nil
	}
	return "", ErrLibreOfficeUnavailable
}

func replaceFile(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := output.ReadFrom(input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}
