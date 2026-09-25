package server

import (
	"bytes"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestPhotoAssetsFollowRenderedView(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	editor := authPermissionsFromCapabilities(authCapPhotosRead|authCapPhotosEdit, authPrincipal{Username: "editor"})
	for _, tc := range []struct {
		name, template string
		data           PageData
		want, absent   []string
	}{
		{"gallery reader", "photos.html", PageData{Active: "photos", PhotoPage: true, Assets: photoPageAssets(false)},
			[]string{"app-photos-map.js", "app-photos-thumbnails.js", "app-photos-lightbox.js", "app-photos.js"}, []string{"app-photos-map-view.js", "app-photos-frame.js", "app-image-groups.js", "app-tags.js"}},
		{"gallery editor", "photos.html", PageData{Active: "photos", PhotoPage: true, Auth: editor, Assets: photoPageAssets(true)},
			[]string{"app-tags.js", "app-image-groups.js"}, []string{"app-photos-map-view.js", "app-photos-frame.js"}},
		{"virtual gallery", "photos.html", PageData{Active: "photos", PhotoPage: true, Auth: editor, Photos: PhotoListingView{Virtual: true}, Assets: photoPageAssets(true)},
			[]string{"app-photos.js"}, []string{"app-image-groups.js"}},
		{"map", "photos.html", PageData{Active: "photos", PhotoPage: true, Auth: editor, PhotoFilter: PhotoFilter{MapView: true}, Assets: photoPageAssets(true)},
			[]string{"app-photos-map.js", "app-photos-map-view.js", "app-photos-lightbox.js"}, []string{"app-image-groups.js", "app-photos-frame.js"}},
		{"frame", "photo_frame.html", PageData{Active: "photos", PhotoFrame: true, Assets: photoFrameAssets()},
			[]string{"app-photos-media.js", "app-photos-frame.js"}, []string{"app-photos.js", "app-photos-map.js", "app-photos-map-view.js", "app-photos-lightbox.js", "app-photos-thumbnails.js", "app-image-groups.js"}},
		{"people overview", "people.html", PageData{Active: "photos", Assets: PageAssets{Explicit: true}},
			[]string{"app-people.js"}, []string{"app-photos-media.js", "app-photos.js", "app-photos-map.js", "app-photos-frame.js", "app-image-groups.js"}},
		{"person detail", "people.html", PageData{Active: "photos", People: photos.PeoplePage{PersonID: 1}, Assets: photoPageAssets(false)},
			[]string{"app-photos-lightbox.js", "app-photos.js", "app-person-detail.js"}, []string{"app-photos-map-view.js", "app-photos-frame.js", "app-image-groups.js"}},
		{"person folders", "person_folder.html", PageData{Active: "photos", Auth: editor, Assets: PageAssets{Explicit: true}},
			[]string{"app-person-folder.js"}, []string{"app-photos.js", "app-photos-map.js", "app-photos-lightbox.js"}},
		{"image group", "image_group.html", PageData{Active: "photos", Auth: editor, ImageGroup: ImageGroupView{ID: 1}, Assets: photoPageAssets(false)},
			[]string{"app-image-groups.js", "app-photos.js"}, []string{"app-photos-frame.js", "app-photos-map-view.js"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := templates.ExecuteTemplate(&out, tc.template, tc.data); err != nil {
				t.Fatal(err)
			}
			html := out.String()
			for _, name := range tc.want {
				if strings.Count(html, "/static/"+name) != 1 {
					t.Errorf("expected asset exactly once: %s", name)
				}
			}
			for _, name := range tc.absent {
				if strings.Contains(html, "/static/"+name) {
					t.Errorf("unneeded asset: %s", name)
				}
			}
			if strings.Contains(html, "/static/app-photos-map-view.js") && strings.Index(html, "/static/app-photos-map.js") > strings.Index(html, "/static/app-photos-map-view.js") {
				t.Fatal("map primitives must load first")
			}
		})
	}
}
