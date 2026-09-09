package photos

import (
	"context"
	"database/sql"
	"errors"
)

// PrepareFacePhoto queues only the requested image. It does not enable the
// background worker or scan unrelated gallery folders.
func (l *Library) PrepareFacePhoto(ctx context.Context, path, model string) (FaceJob, error) {
	var job FaceJob
	if !validGroupPhotoPath(path, false) || !CanThumbnail(path) {
		return job, ErrLabelInvalid
	}
	if err := l.refreshGroupPhoto(ctx, path); err != nil {
		return job, err
	}
	media, err := l.MediaContext(ctx, path)
	if err != nil {
		return job, err
	}
	if media.AdminOnly {
		return job, ErrAdminOnly()
	}
	if media.Type != MediaTypeImage {
		return job, ErrLabelInvalid
	}
	job = FaceJob{Path: path, Size: media.SizeBytes, ModTime: media.ModTime.UnixNano(), XMP: media.XMPFingerprint, Model: model}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return job, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET model=? WHERE id=1`, model); err != nil {
		return job, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO photo_face_jobs(path,directory,source_size,source_mtime,source_xmp,model) VALUES(?,?,?,?,?,?)
 ON CONFLICT(path) DO UPDATE SET source_size=excluded.source_size,source_mtime=excluded.source_mtime,source_xmp=excluded.source_xmp,model=excluded.model,status='queued',attempts=0,retry_at=0,error=''`, path, media.Directory, job.Size, job.ModTime, job.XMP, model)
	if err != nil {
		return job, err
	}
	return job, tx.Commit()
}

// PhotoFaces also serves valid photos that have not been analyzed or contain no
// faces. The group-review endpoint keeps its existing nonempty-group contract.
func (l *Library) PhotoFaces(ctx context.Context, path string) (GroupPhoto, error) {
	if err := l.refreshGroupPhoto(ctx, path); err != nil {
		return GroupPhoto{}, err
	}
	photo, err := readGroupPhoto(ctx, l.index.db, path)
	if errors.Is(err, sql.ErrNoRows) && len(photo.Faces) == 0 {
		return photo, nil
	}
	return photo, err
}
