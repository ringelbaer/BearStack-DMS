package photos

import "context"

func (l *Library) saveMedia(media Media) {
	if err := l.saveMediaContext(context.Background(), media); err != nil {
		l.logWriteError("photo media cache update failed", media.Path, err)
	}
}
