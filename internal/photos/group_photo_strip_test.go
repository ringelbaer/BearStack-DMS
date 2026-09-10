package photos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupPhotoStripPaginationAndAnchor(t *testing.T) {
	ctx := context.Background()
	paths := []string{}
	for i := range 70 {
		paths = append(paths, fmt.Sprintf("%03d.jpg", i))
	}
	l := faceLibrary(t, paths...)
	for range paths {
		finishGroupPhoto(t, l, 6, 0)
	}
	first, err := l.GroupPhotoPreviews(ctx, "", "", "", 5)
	if err != nil || len(first.Photos) != 32 || first.HasPrevious || !first.HasNext || first.Before != paths[0] || first.After != paths[31] {
		t.Fatalf("first=%+v %v", first, err)
	}
	second, err := l.GroupPhotoPreviews(ctx, "", first.After, "", 5)
	if err != nil || len(second.Photos) != 32 || !second.HasPrevious || !second.HasNext || second.Before != paths[32] {
		t.Fatalf("second=%+v %v", second, err)
	}
	last, err := l.GroupPhotoPreviews(ctx, "", second.After, "", 5)
	if err != nil || len(last.Photos) != 6 || last.HasNext {
		t.Fatalf("last=%+v %v", last, err)
	}
	back, err := l.GroupPhotoPreviews(ctx, "", "", second.Before, 5)
	if err != nil || len(back.Photos) != 32 || back.HasPrevious || !back.HasNext || back.Before != first.Before || back.After != first.After {
		t.Fatalf("back=%+v %v", back, err)
	}
	centered, err := l.GroupPhotoPreviews(ctx, paths[35], "", "", 5)
	if err != nil || len(centered.Photos) != 33 || centered.Photos[16].Path != paths[35] || !centered.HasPrevious || !centered.HasNext {
		t.Fatalf("centered=%+v %v", centered, err)
	}
	photo, err := l.GroupPhoto(ctx, paths[35])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.IgnoreGroupPhoto(ctx, photo.Path, photo.Revision); err != nil {
		t.Fatal(err)
	}
	centered, err = l.GroupPhotoPreviews(ctx, photo.Path, "", "", 5)
	if err != nil || centered.Photos[16].Path != photo.Path || centered.Photos[16].Remaining != 0 {
		t.Fatalf("processed current evicted: %+v %v", centered, err)
	}
	next, err := l.GroupPhotoPreviews(ctx, "", paths[34], "", 5)
	if err != nil || next.Photos[0].Path != paths[36] {
		t.Fatalf("processed queued photo retained: %+v %v", next, err)
	}
	encoded, _ := json.Marshal(centered)
	if strings.Contains(string(encoded), "embedding") || strings.Contains(string(encoded), "faces") {
		t.Fatalf("heavy face data exposed: %s", encoded)
	}
	for _, args := range [][3]string{{"../bad", "", ""}, {"", "/bad", ""}, {"", "", "a//bad"}, {"a.jpg", "b.jpg", ""}, {"", "a.jpg", "b.jpg"}} {
		if _, err := l.GroupPhotoPreviews(ctx, args[0], args[1], args[2], 5); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("accepted %v: %v", args, err)
		}
	}
	for _, minimum := range []int{-1, 256} {
		if _, err := l.GroupPhotoPreviews(ctx, "", "", "", minimum); !errors.Is(err, ErrLabelInvalid) {
			t.Fatal(err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.GroupPhotoPreviews(cancelled, "", "", "", 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestGroupPhotoStripSkipsPrivateMissingAndBelowThreshold(t *testing.T) {
	for _, scenario := range []string{"private", "deleted", "replaced", "threshold"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a/one.jpg", "b/two.jpg")
			_, a := finishGroupPhoto(t, l, 6, 0)
			_, b := finishGroupPhoto(t, l, 6, 20)
			switch scenario {
			case "private":
				if err := os.WriteFile(filepath.Join(l.Root(), "a/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := os.Remove(filepath.Join(l.Root(), a.Path)); err != nil {
					t.Fatal(err)
				}
			case "replaced":
				if err := os.WriteFile(filepath.Join(l.Root(), a.Path), []byte("replaced image"), 0600); err != nil {
					t.Fatal(err)
				}
			case "threshold":
				if err := l.RenamePerson(ctx, a.Faces[0].PersonID, "Ada"); err != nil {
					t.Fatal(err)
				}
			}
			strip, err := l.GroupPhotoPreviews(ctx, "", "", "", 5)
			if err != nil || len(strip.Photos) != 1 || strip.Photos[0].Path != b.Path {
				t.Fatalf("strip=%+v %v", strip, err)
			}
			strip, err = l.GroupPhotoPreviews(ctx, "", "", "z.jpg", 5)
			if err != nil || len(strip.Photos) != 1 || strip.Photos[0].Path != b.Path {
				t.Fatalf("reverse strip=%+v %v", strip, err)
			}
		})
	}
}
