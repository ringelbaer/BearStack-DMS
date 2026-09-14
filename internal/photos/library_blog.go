// Datei liest Blogbeitraege aus dem Fotoverzeichnis und verbindet sie mit dem Fotoindex.
package photos

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bearstack/internal/photos/blogcontent"
)

// Blog reads supported text content using the same renderer and size limit as
// the browser. Resolve rejects symlink traversal; access is checked before IO.
func (l *Library) Blog(ctx context.Context, path string, includeAdminOnly bool) (BlogPost, error) {
	if err := ctx.Err(); err != nil {
		return BlogPost{}, err
	}
	rel, err := CleanPath(path)
	if err != nil {
		return BlogPost{}, err
	}
	if kind, ok := supportedKind(rel); !ok || kind != MediaTypeBlog {
		return BlogPost{}, os.ErrNotExist
	}
	for _, part := range strings.Split(rel, "/") {
		if ignoredName(part) {
			return BlogPost{}, os.ErrNotExist
		}
	}
	if !includeAdminOnly {
		private, err := l.FolderAdminOnly(parentPath(rel))
		if err != nil {
			return BlogPost{}, err
		}
		if private {
			return BlogPost{}, ErrAdminOnly()
		}
	}
	return l.blogFromPathData(rel)
}

func (l *Library) blogFromPath(rel string) (BlogPost, error) {
	post, err := l.blogFromPathData(rel)
	if err != nil {
		return BlogPost{}, err
	}
	l.saveBlog(post)
	return post, nil
}

func (l *Library) blogFromPathData(rel string) (BlogPost, error) {
	abs, err := l.Resolve(rel)
	if err != nil {
		return BlogPost{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return BlogPost{}, err
	}
	adminOnly := l.directoryAdminOnly(parentPath(rel))
	post, _, err := l.blogFromPathInfo(rel, abs, info, nil, adminOnly)
	if err != nil {
		return BlogPost{}, err
	}
	return post, nil
}

func (l *Library) blogFromPathInfo(rel, abs string, info os.FileInfo, cache map[string]cachedBlogRow, adminOnly bool) (BlogPost, bool, error) {
	if !info.Mode().IsRegular() {
		return BlogPost{}, false, os.ErrNotExist
	}
	if row, ok := cache[rel]; ok && row.ModTimeUnixNano == info.ModTime().UnixNano() && (row.AdminOnly != 0) == adminOnly {
		return blogFromCachedRow(row, info.ModTime()), false, nil
	}
	file, err := os.Open(abs)
	if err != nil {
		return BlogPost{}, false, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBlogBytes))
	if err != nil {
		return BlogPost{}, false, err
	}
	post := BlogPost{
		Name:      filepath.Base(filepath.FromSlash(rel)),
		Path:      rel,
		AdminOnly: adminOnly,
		Date:      blogcontent.Date(raw),
		ModTime:   info.ModTime(),
	}
	post.Text, post.HTML = blogcontent.Render(rel, raw)
	if row, ok := cache[rel]; ok {
		post.Tags = tagsFromJSON(row.Tags)
	} else if tags, ok := l.blogTags(rel); ok {
		post.Tags = tags
	}
	return post, true, nil
}

func blogFromCachedRow(row cachedBlogRow, modTime time.Time) BlogPost {
	post := BlogPost{
		Name:      row.Name,
		Path:      row.Path,
		Tags:      tagsFromJSON(row.Tags),
		AdminOnly: row.AdminOnly != 0,
		Text:      row.Text,
		ModTime:   modTime,
	}
	_, post.HTML = blogcontent.Render(row.Name, []byte(row.Text))
	if row.Date != "" {
		if parsed, err := time.Parse("2006-01-02", row.Date); err == nil {
			post.Date = &parsed
		}
	}
	return post
}
