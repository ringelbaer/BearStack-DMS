package server

import (
	"context"
	"testing"
	"time"
)

func TestBackgroundWorkersFactoryWiresServerWorkers(t *testing.T) {
	t.Parallel()

	server := &Server{}
	workers := server.BackgroundWorkers()

	if workers.server != server {
		t.Fatal("background workers did not keep server reference")
	}
	if workers.runDocumentPostImport == nil ||
		workers.runOCRQueue == nil ||
		workers.runMailImport == nil ||
		workers.runTrashRetention == nil ||
		workers.runPhotoIndexWorker == nil ||
		workers.runPhotoThumbnailWorker == nil ||
		workers.ensureThumbnails == nil {
		t.Fatal("background workers factory returned missing worker callbacks")
	}
}

func TestBackgroundWorkersStartLaunchesPhotoWorkersAndSetsContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := &Server{}
	indexStarted := make(chan struct{}, 1)
	thumbnailStarted := make(chan struct{}, 1)
	documentStarted := make(chan struct{}, 1)
	ocrStarted := make(chan struct{}, 1)
	mailStarted := make(chan struct{}, 1)
	trashStarted := make(chan struct{}, 1)

	workers := BackgroundWorkers{
		server: server,
		runPhotoIndexWorker: func(runCtx context.Context) {
			if runCtx != ctx {
				t.Errorf("photo index worker context mismatch")
			}
			indexStarted <- struct{}{}
		},
		runPhotoThumbnailWorker: func(runCtx context.Context) {
			if runCtx != ctx {
				t.Errorf("photo thumbnail worker context mismatch")
			}
			thumbnailStarted <- struct{}{}
		},
		runDocumentPostImport: func(runCtx context.Context) {
			if runCtx != ctx {
				t.Errorf("document post-import context mismatch")
			}
			documentStarted <- struct{}{}
		},
		runOCRQueue: func(runCtx context.Context) {
			if runCtx != ctx {
				t.Errorf("ocr queue context mismatch")
			}
			ocrStarted <- struct{}{}
		},
		runMailImport: func(runCtx context.Context) {
			if runCtx != ctx {
				t.Errorf("mail import context mismatch")
			}
			mailStarted <- struct{}{}
		},
		runTrashRetention: func(runCtx context.Context) {
			if runCtx != ctx {
				t.Errorf("trash retention context mismatch")
			}
			trashStarted <- struct{}{}
		},
	}

	workers.Start(ctx)

	waitStarted(t, "photo index worker", indexStarted)
	waitStarted(t, "photo thumbnail worker", thumbnailStarted)
	waitStarted(t, "document post-import worker", documentStarted)
	waitStarted(t, "ocr worker", ocrStarted)
	waitStarted(t, "mail import worker", mailStarted)
	waitStarted(t, "trash retention worker", trashStarted)

	if got := server.backgroundJobContext(); got != ctx {
		t.Fatalf("background job context mismatch")
	}
}

func TestBackgroundWorkersStartSkipsDelayedThumbnailWarmupAfterCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ensureCalled := make(chan struct{}, 1)
	workers := BackgroundWorkers{
		ensureThumbnails: func(context.Context) error {
			ensureCalled <- struct{}{}
			return nil
		},
	}

	workers.Start(ctx)
	cancel()

	select {
	case <-ensureCalled:
		t.Fatal("thumbnail warmup ran despite immediate cancellation")
	case <-time.After(250 * time.Millisecond):
	}
}

func waitStarted(t *testing.T, name string, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatalf("%s did not start", name)
	}
}
