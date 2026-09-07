package server

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/repository"
)

func TestHandleSaveSettingsSections(t *testing.T) {
	for _, section := range []string{"general", "documents", "invalid"} {
		t.Run(section, func(t *testing.T) {
			ctx := context.Background()
			dbPath := filepath.Join(t.TempDir(), "settings.db")
			repo, err := repository.Open(ctx, dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			templates, err := parseTemplates()
			if err != nil {
				t.Fatal(err)
			}
			s := &Server{repo: repo, templates: templates, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			want := map[string]string{
				appNameSettingKey: "Archiv", themeModeSettingKey: themeModeDesign2,
				tagDisplayModeSettingKey: tagDisplayModeUpper, homePageSettingKey: homePageCloud,
				desktopPreviewModeSettingKey: desktopPreviewModeInline, documentCloudEnabledSettingKey: "1",
				trashRetentionDaysSettingKey: "60", folderTagMinDocumentsSettingKey: "12",
			}
			for key, value := range want {
				if err := repo.SaveSetting(ctx, key, value); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.appName(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := s.renderSettings(ctx); err != nil {
				t.Fatal(err)
			}
			// An unrelated general save must not trigger retention cleanup.
			id, err := repo.CreateDocument(ctx, document.Document{OriginalName: "old.pdf", StoredPath: "old.pdf", SHA256: "old", MIMEType: "application/pdf"})
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.SoftDelete(ctx, id); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.ExecContext(ctx, "UPDATE documents SET deleted_at = ? WHERE id = ?", time.Now().UTC().AddDate(0, 0, -100).Format(time.RFC3339), id)
			db.Close()
			if err != nil {
				t.Fatal(err)
			}

			// Even submitted fields from another section must be ignored.
			form := url.Values{
				"settings_section": {section}, "app_name": {"Familienarchiv"}, "theme_mode": {themeModeDefault},
				"tag_display_mode": {tagDisplayModeFirst}, "home_page": {homePageCloud},
				"desktop_preview_mode": {desktopPreviewModeModal}, "folder_tag_min_documents": {"17"}, "trash_retention_days": {"0"},
			}
			req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			s.handleSaveSettings(rec, req)
			if section == "invalid" {
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("invalid section status = %d", rec.Code)
				}
			} else {
				if rec.Code != http.StatusSeeOther {
					t.Fatalf("save status = %d: %s", rec.Code, rec.Body.String())
				}
				path := "/settings"
				if section == "general" {
					path += "/general"
					want[appNameSettingKey] = "Familienarchiv"
					want[themeModeSettingKey] = themeModeDefault
					want[tagDisplayModeSettingKey] = tagDisplayModeFirst
				} else {
					want[desktopPreviewModeSettingKey] = desktopPreviewModeModal
					want[documentCloudEnabledSettingKey] = "0"
					want[folderTagMinDocumentsSettingKey] = "17"
					want[trashRetentionDaysSettingKey] = "0"
				}
				if location := rec.Header().Get("Location"); !strings.HasPrefix(location, path+"?notice=") {
					t.Fatalf("save redirect = %q", location)
				}
			}
			for key, value := range want {
				if got, _, err := repo.GetSetting(ctx, key); err != nil || got != value {
					t.Errorf("setting %s = %q, want %q (err %v)", key, got, value, err)
				}
			}
			if name, err := s.appName(ctx); err != nil || name != want[appNameSettingKey] {
				t.Errorf("cached name = %q, err %v", name, err)
			}
			cached, err := s.renderSettings(ctx)
			if err != nil || cached.ThemeMode != want[themeModeSettingKey] || cached.TagDisplayMode != want[tagDisplayModeSettingKey] || cached.HomePage != want[homePageSettingKey] || cached.DocumentCloudEnabled != (want[documentCloudEnabledSettingKey] == "1") {
				t.Errorf("cached settings = %+v, err %v", cached, err)
			}
			if total, err := repo.CountDocuments(ctx, document.ListFilter{Trash: true}); err != nil || total != 1 {
				t.Errorf("unrelated save purged trash: total %d, err %v", total, err)
			}
			if section == "documents" {
				page, err := s.resolvedHomePage(ctx, AuthPermissions{CanDocumentsRead: true})
				if err != nil || page != homePageDocuments {
					t.Errorf("disabled cloud must fall back to documents: page %q, err %v", page, err)
				}
			}
		})
	}
}
