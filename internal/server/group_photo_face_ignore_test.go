package server

import (
	"context"
	"fmt"
	"net/url"
	"testing"
)

func TestGroupPhotoInvalidFaceSelectionNeverIgnoresAll(t *testing.T) {
	s, photo := groupPhotoServerFixture(t)
	for _, ids := range [][]string{{""}, {"0"}, {"-1"}, {"abc"}, {"9223372036854775808"}, {"999999999"}, {fmt.Sprint(photo.Faces[0].ID), fmt.Sprint(photo.Faces[1].ID)}} {
		form := url.Values{"path": {photo.Path}, "revision": {photo.Revision}, "face_id": ids}
		w := labelRequest(s, "POST", "/photos/people/groups/ignore", "editor", form.Encode())
		if w.Code != 400 {
			t.Fatalf("invalid face IDs %v: %d %s", ids, w.Code, w.Body.String())
		}
		current, err := s.photos.GroupPhoto(context.Background(), photo.Path)
		if err != nil || current.Revision != photo.Revision || current.Remaining != 6 {
			t.Fatalf("invalid face IDs mutated photo: %+v %v", current, err)
		}
	}
}
