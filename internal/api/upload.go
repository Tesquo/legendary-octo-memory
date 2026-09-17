package api

import (
	"io"
	"net/http"
	"os"

	"github.com/Tesquo/legendary-octo-memory/internal/processing"
	"github.com/Tesquo/legendary-octo-memory/internal/storage"
	"github.com/google/uuid"
)

// UploadHandler stores an uploaded file, records it in the library, kicks off a
// background thumbnail job, and queues the file for preparation.
func (s *Server) UploadHandler(w http.ResponseWriter, r *http.Request) {
	// Cap the request body so a single upload cannot exhaust disk or memory.
	r.Body = http.MaxBytesReader(w, r.Body, s.Config.MaxUploadBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file upload error", http.StatusBadRequest)
		return
	}
	defer file.Close()

	id := uuid.New().String()

	if err := os.MkdirAll(thumbDir(id), 0755); err != nil {
		http.Error(w, "cannot create media dir", http.StatusInternalServerError)
		return
	}

	dstPath := sourcePath(id, header.Filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, "cannot save file", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		http.Error(w, "cannot write file", http.StatusInternalServerError)
		return
	}
	dst.Close()

	md, err := processing.ExtractMetadata(dstPath)
	if err != nil {
		http.Error(w, "metadata error", http.StatusInternalServerError)
		return
	}

	media := &storage.Media{
		ID:         id,
		Filename:   header.Filename,
		Status:     storage.StatusImported,
		Duration:   md.Duration,
		Width:      md.Width,
		Height:     md.Height,
		VideoCodec: md.VideoCodec,
		AudioCodec: md.AudioCodec,
	}
	if err := s.Store.Insert(media); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Thumbnail generation is independent of the transcode pipeline. When it
	// finishes we publish a dedicated event so clients refresh and pick up the
	// new preview immediately, rather than waiting for an unrelated refresh.
	go func() {
		if _, err := processing.GenerateThumbnail(dstPath, thumbDir(id)); err != nil {
			return
		}
		s.Store.SetThumbnail(id, thumbPath(id))
		s.Hub.Publish(id, processing.ProgressEvent{
			MediaID: id,
			Stage:   processing.StageThumbnail,
			Message: "thumbnail ready",
		})
	}()

	s.enqueueJob(id, dstPath, md.Duration)

	writeJSON(w, http.StatusOK, map[string]string{
		"id":       id,
		"filename": header.Filename,
		"status":   "uploaded",
	})
}

// enqueueJob classifies a source file and submits it to the ffmpeg pipeline.
// Files the browser can already play skip the queue entirely and are marked
// ready immediately.
func (s *Server) enqueueJob(id, srcPath string, duration float64) {
	md, err := processing.ExtractMetadata(srcPath)
	if err != nil {
		s.Store.SetStatus(id, storage.StatusFailed)
		s.Hub.Publish(id, processing.ProgressEvent{
			MediaID: id,
			Stage:   processing.StageFailed,
			Message: "metadata error",
		})
		return
	}

	strategy := processing.Classify(srcPath, md)

	if strategy == processing.StrategyPlayable {
		// No transcoding required: the source file is already playable.
		s.Store.SetPlayable(id, srcPath, storage.StatusReady)
		s.Hub.Publish(id, processing.ProgressEvent{
			MediaID:  id,
			Stage:    processing.StageDone,
			Strategy: strategy.String(),
			Percent:  100,
			Message:  "ready",
		})
		return
	}

	s.Store.SetStatus(id, storage.StatusProcessing)
	s.Pipeline.Submit(processing.Job{
		MediaID:    id,
		SourcePath: srcPath,
		OutputPath: playablePath(id),
		Strategy:   strategy,
		Duration:   duration,
	})
}
