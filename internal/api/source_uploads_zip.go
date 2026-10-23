package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"

	"github.com/GLINCKER/levelrail/internal/sourceupload"
)

// isZipUpload reports whether the request body is a zip rather than the CLI's gzip tar.
func isZipUpload(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	return mt == "application/zip" || mt == "application/x-zip-compressed"
}

// convertZipBody spools a zip body to disk (zip needs random access), validates it and
// returns the equivalent gzip tar. It writes the HTTP error itself and returns ok false on failure.
func (rt *Router) convertZipBody(w http.ResponseWriter, r *http.Request, limit int64) (body io.Reader, res sourceupload.ZipResult, cleanup func(), ok bool) {
	var files []*os.File
	cleanup = func() {
		for _, f := range files {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}
	res, status, msg := rt.spoolZip(r, limit, &files)
	if status != 0 {
		cleanup()
		writeError(w, status, msg)
		return nil, sourceupload.ZipResult{}, func() {}, false
	}
	return files[len(files)-1], res, cleanup, true
}

// spoolZip appends two temp files to files (the zip, then the converted tar) and returns an HTTP status and message on failure.
func (rt *Router) spoolZip(r *http.Request, limit int64, files *[]*os.File) (sourceupload.ZipResult, int, string) {
	zipFile, err := os.CreateTemp("", "drop-zip-*")
	if err != nil {
		rt.logger.Error("api: zip upload: temp file failed", slog.String("error", err.Error()))
		return sourceupload.ZipResult{}, http.StatusInternalServerError, "internal error"
	}
	*files = append(*files, zipFile)
	n, err := io.Copy(zipFile, io.LimitReader(r.Body, limit+1))
	if err != nil {
		return sourceupload.ZipResult{}, http.StatusBadRequest, "could not read the upload, try again"
	}
	if n > limit {
		return sourceupload.ZipResult{}, http.StatusRequestEntityTooLarge, fmt.Sprintf("zip is larger than the %d byte limit (%s)", limit, envUploadMaxBytes)
	}
	tgz, err := os.CreateTemp("", "drop-tgz-*")
	if err != nil {
		rt.logger.Error("api: zip upload: temp file failed", slog.String("error", err.Error()))
		return sourceupload.ZipResult{}, http.StatusInternalServerError, "internal error"
	}
	*files = append(*files, tgz)
	res, err := sourceupload.ConvertZip(zipFile, n, tgz, uploadLimits())
	switch {
	case errors.Is(err, sourceupload.ErrTooLarge):
		return res, http.StatusRequestEntityTooLarge, "site is too large once unpacked: " + err.Error()
	case errors.Is(err, sourceupload.ErrUnsafeArchive):
		rt.logger.Warn("api: zip upload rejected", slog.String("error", err.Error()))
		return res, http.StatusBadRequest, "zip rejected: " + err.Error()
	case errors.Is(err, sourceupload.ErrEmptyArchive):
		return res, http.StatusBadRequest, "the zip has no files to publish"
	case err != nil:
		return res, http.StatusBadRequest, "could not read the zip: " + err.Error()
	}
	if _, err := tgz.Seek(0, io.SeekStart); err != nil {
		return res, http.StatusInternalServerError, "internal error"
	}
	return res, 0, ""
}
