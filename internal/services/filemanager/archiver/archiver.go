package archiver

import (
	"archive/zip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
)

const (
	skippedReportName = "_SKIPPED.txt"
	flushInterval     = 64 * 1024
)

type Options struct {
	CompressLevel int
}

type Result struct {
	BytesWritten uint64
}

type Archiver struct {
	lister   FileLister
	streamer FileStreamer
	logger   *slog.Logger
}

func NewArchiver(lister FileLister, streamer FileStreamer, logger *slog.Logger) *Archiver {
	if logger == nil {
		logger = slog.Default()
	}

	return &Archiver{
		lister:   lister,
		streamer: streamer,
		logger:   logger,
	}
}

func (a *Archiver) BuildManifest(
	ctx context.Context,
	node *domain.Node,
	rootAbsPath string,
	limits Limits,
) (*Manifest, error) {
	return BuildManifest(ctx, a.lister, node, rootAbsPath, limits)
}

func (a *Archiver) WriteArchive(
	ctx context.Context,
	w io.Writer,
	node *domain.Node,
	manifest *Manifest,
	opts Options,
) (*Result, error) {
	cw := &countingWriter{w: w}
	zw := zip.NewWriter(cw)

	if opts.CompressLevel > 0 {
		zw.RegisterCompressor(zip.Deflate, deflateCompressorFor(opts.CompressLevel))
	}

	flusher := flusherFor(w)

	// On error paths the zip writer is deliberately left unclosed: closing it would append a valid
	// central directory and turn a partial stream into an archive that opens but silently lacks files.
	for _, entry := range manifest.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if err := a.writeEntry(ctx, zw, node, entry, opts); err != nil {
			return nil, errors.WithMessage(err, "write entry "+entry.RelPath)
		}

		if flusher != nil && cw.bytesWritten >= flushInterval {
			flusher()
			cw.bytesWritten = 0
		}
	}

	if len(manifest.Skipped) > 0 {
		if err := writeSkippedReport(zw, manifest.Skipped); err != nil {
			return nil, errors.WithMessage(err, "write skipped report")
		}
	}

	if err := zw.Close(); err != nil {
		return nil, errors.Wrap(err, "close zip writer")
	}

	if flusher != nil {
		flusher()
	}

	return &Result{BytesWritten: cw.totalBytesWritten}, nil
}

func (a *Archiver) writeEntry(
	ctx context.Context,
	zw *zip.Writer,
	node *domain.Node,
	entry Entry,
	opts Options,
) error {
	header := &zip.FileHeader{
		Name:     entry.RelPath,
		Modified: time.Unix(safeUnix(entry.ModTime), 0),
	}
	header.SetMode(os.FileMode(entry.Mode))

	switch entry.Type {
	case daemon.FileTypeDir:
		header.Method = zip.Store
		_, err := zw.CreateHeader(header)

		return err

	case daemon.FileTypeSymlink:
		header.SetMode(os.FileMode(entry.Mode) | os.ModeSymlink)
		header.Method = zip.Store
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		target := entry.SymlinkTarget
		if target == "" {
			details, lookupErr := a.lister.GetFileInfo(ctx, node, entry.AbsPath)
			if lookupErr == nil && details != nil {
				target = details.SymlinkTarget
			}
		}
		_, err = io.WriteString(w, target)

		return err

	case daemon.FileTypeFile:
		if opts.CompressLevel > 0 {
			header.Method = zip.Deflate
		} else {
			header.Method = zip.Store
		}
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		stream, err := a.streamer.DownloadStream(ctx, node, entry.AbsPath)
		if err != nil {
			return errors.Wrap(err, "open file stream")
		}
		defer func() {
			if closeErr := stream.Close(); closeErr != nil {
				a.logger.WarnContext(ctx, "failed to close file stream",
					slog.String("path", entry.AbsPath), slog.String("error", closeErr.Error()))
			}
		}()

		if _, err := io.Copy(w, stream); err != nil {
			return errors.Wrap(err, "copy file content")
		}

		return nil

	default:
		return nil
	}
}

func writeSkippedReport(zw *zip.Writer, skipped []string) error {
	header := &zip.FileHeader{
		Name:     skippedReportName,
		Method:   zip.Store,
		Modified: time.Now(),
	}
	header.SetMode(0o644)

	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	body := strings.Join(skipped, "\n") + "\n"
	_, err = io.WriteString(w, body)

	return err
}

func flusherFor(w io.Writer) func() {
	if rw, ok := w.(http.ResponseWriter); ok {
		rc := http.NewResponseController(rw)

		return func() {
			_ = rc.Flush()
		}
	}
	if f, ok := w.(http.Flusher); ok {
		return f.Flush
	}

	return nil
}

func safeUnix(ts uint64) int64 {
	if ts == 0 || ts > (1<<62) {
		return time.Now().Unix()
	}

	return int64(ts)
}

type countingWriter struct {
	w                 io.Writer
	bytesWritten      int
	totalBytesWritten uint64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.bytesWritten += n
	if n > 0 {
		cw.totalBytesWritten += uint64(n)
	}

	return n, err
}
