package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"bearstack/internal/account"
	"bearstack/internal/photos"
	"bearstack/internal/photos/photopath"
	"bearstack/internal/transfers"
)

type photoTransferSource struct{ library *photos.Library }

func transferOptions(selection transfers.Selection) photos.ListOptions {
	return photos.ListOptions{Path: selection.Path, Query: selection.Query, MediaType: selection.MediaType, GPSOnly: selection.GPSOnly, IncludeAdminOnly: selection.IncludeAdminOnly, Recursive: true, LeanMetadata: true, SkipFolders: true, SkipBlogs: true, PageSize: 1}
}
func (source photoTransferSource) Describe(ctx context.Context, selection transfers.Selection) (transfers.SelectionInfo, error) {
	clean, err := photos.CleanPath(selection.Path)
	if err != nil || clean != selection.Path || len(selection.Query) > 4096 {
		return transfers.SelectionInfo{}, transfers.Fail(transfers.Invalid, "Ungültige Fotoauswahl")
	}
	switch selection.MediaType {
	case "", photos.MediaTypeImage, photos.MediaTypeVideo, photos.MediaTypeAudio:
	default:
		return transfers.SelectionInfo{}, transfers.Fail(transfers.Invalid, "Ungültiger Medientyp")
	}
	if selection.Paths != nil {
		if len(selection.Paths) == 0 || len(selection.Paths) > 5000 {
			return transfers.SelectionInfo{}, transfers.Fail(transfers.Invalid, "Bitte zwischen 1 und 5000 Medien auswählen")
		}
		for _, value := range selection.Paths {
			canonical, err := photos.CleanPath(value)
			if err != nil || canonical != value || canonical == "" {
				return transfers.SelectionInfo{}, transfers.Fail(transfers.Invalid, "Ungültiger Fotopfad")
			}
		}
		return transfers.SelectionInfo{Name: "Auswahl", Target: "Auswahl", Virtual: true}, nil
	}

	virtual := strings.TrimSpace(selection.Query) != "" || photos.IsPeopleFolder(clean)
	if !virtual && len(strings.Split(clean, "/")) < 2 {
		return transfers.SelectionInfo{}, transfers.Fail(transfers.Invalid, "Upload erst ab der dritten Ebene")
	}
	if photos.IsPeopleFolder(clean) && len(strings.Split(clean, "/")) != 3 {
		return transfers.SelectionInfo{}, transfers.Fail(transfers.Invalid, "Bitte einen Personen-Fotoordner öffnen")
	}
	opts := transferOptions(selection)
	opts.SkipMedia = true
	listing, err := source.library.List(ctx, opts)
	if err != nil {
		return transfers.SelectionInfo{}, err
	}
	name, target := photos.MediaFolderName(path.Join(clean, "_")), path.Base(clean)
	if virtual {
		if photos.IsPeopleFolder(clean) {
			if len(listing.Breadcrumbs) == 0 {
				return transfers.SelectionInfo{}, transfers.Fail(transfers.Invalid, "Personenordner nicht gefunden")
			}
			name = "Person – " + listing.Breadcrumbs[len(listing.Breadcrumbs)-1].Name
		} else {
			name = "Suche – " + selection.Query
		}
		target = strings.NewReplacer("/", "-", "\\", "-", ":", "-", "\x00", "", "\r", " ", "\n", " ").Replace(name)
		target = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, target)
		if len(target) > 180 {
			hash := sha256.Sum256([]byte(target))
			target = target[:160]
			for !utf8.ValidString(target) {
				target = target[:len(target)-1]
			}
			target += " – " + hex.EncodeToString(hash[:4])
		}
	}
	return transfers.SelectionInfo{Name: name, Target: target, Virtual: virtual}, nil
}
func (source photoTransferSource) Walk(ctx context.Context, selection transfers.Selection, emit func(transfers.SourceItem) error) error {
	info, err := source.Describe(ctx, selection)
	if err != nil {
		return err
	}
	opts := transferOptions(selection)
	if !info.Virtual {
		opts.Query = ""
		opts.MediaType = ""
		opts.GPSOnly = false
	}
	emitMedia := func(media photos.Media) error {
		if !selection.IncludeAdminOnly || photos.IsPeopleFolder(selection.Path) {
			private, err := source.library.MediaAdminOnly(media.Path)
			if err != nil {
				return err
			}
			if private {
				return nil
			}
		}
		full, err := source.library.Resolve(media.Path)
		if err != nil {
			return err
		}
		stat, err := os.Stat(full)
		if err != nil {
			return err
		}
		if !stat.Mode().IsRegular() {
			return transfers.Fail(transfers.Invalid, "Quelle ist keine reguläre Datei")
		}
		rel := media.Path
		if !info.Virtual {
			prefix := selection.Path + "/"
			if !strings.HasPrefix(rel, prefix) {
				return transfers.Fail(transfers.Invalid, "Foto liegt außerhalb der Auswahl")
			}
			rel = strings.TrimPrefix(rel, prefix)
		}
		return emit(transfers.SourceItem{Path: media.Path, DisplayPath: photos.MediaDisplayPath(media.Path), Relative: strings.Split(rel, "/"), Size: stat.Size(), Modified: stat.ModTime().UnixNano()})
	}
	if selection.Paths != nil {
		return source.library.WalkExportSelection(ctx, selection.Paths, emitMedia)
	}
	return source.library.WalkExportMedia(ctx, opts, emitMedia)
}
func (source photoTransferSource) Open(ctx context.Context, selection transfers.Selection, item transfers.SourceItem) (transfers.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !selection.IncludeAdminOnly || photos.IsPeopleFolder(selection.Path) {
		private, err := source.library.MediaAdminOnly(item.Path)
		if err != nil {
			return nil, err
		}
		if private {
			return nil, transfers.Fail(transfers.Permission, "Foto ist inzwischen ausgeblendet")
		}
	}
	if err := photopath.RejectSymlinkPath(source.library.Root(), item.Path); err != nil {
		return nil, transfers.Fail(transfers.Invalid, "Foto enthält einen symbolischen Pfad")
	}
	root, err := os.OpenRoot(source.library.Root())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(item.Path)
	if err != nil {
		return nil, transfers.Fail(transfers.Invalid, "Quelldatei nicht mehr verfügbar")
	}
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != item.Size || stat.ModTime().UnixNano() != item.Modified {
		file.Close()
		return nil, transfers.Fail(transfers.Conflict, "Quelldatei wurde seit der Vorschau geändert")
	}
	return file, nil
}
func (s *Server) transferAdministrator(ctx context.Context, actor string) bool {
	if s.auth == nil {
		return false
	}
	var principal authPrincipal
	if json.Unmarshal([]byte(actor), &principal) != nil {
		return false
	}
	snapshot := s.auth.snapshot.Load()
	if snapshot == nil {
		return false
	}
	credential := snapshot.bySubject[authSubjectKey(principal.Source, principal.Subject)]
	return credential != nil && credential.enabled && credential.revision == principal.Revision && account.IsAdministratorRole(credential.role)
}
