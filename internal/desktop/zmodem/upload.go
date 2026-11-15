package zmodem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// Upload retains an opened source, so replacing its path during a native
// selection or transfer cannot silently switch the transmitted file.
// The sender owns and closes every returned Reader, including skipped files.
type Upload struct {
	Offer  FileOffer
	Reader ReadAtCloser
}
type ReadAtCloser interface {
	io.ReaderAt
	io.Closer
}

func OpenUploads(ctx context.Context, paths []string) ([]Upload, error) {
	if len(paths) == 0 {
		return nil, context.Canceled
	}
	if len(paths) > 4096 {
		return nil, errors.New("too many ZMODEM upload files")
	}
	files := make([]Upload, 0, len(paths))
	fail := func(err error) ([]Upload, error) {
		for _, file := range files {
			_ = file.Reader.Close()
		}
		return nil, err
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return fail(context.Cause(ctx))
		}
		if !filepath.IsAbs(path) {
			return fail(errors.New("choose absolute ZMODEM upload paths"))
		}
		// Check before opening to avoid blocking on a deliberately selected FIFO.
		info, err := os.Stat(path)
		if err != nil {
			return fail(err)
		}
		if !info.Mode().IsRegular() {
			return fail(errors.New("select regular files for ZMODEM upload"))
		}
		file, err := openUpload(path)
		if err != nil {
			return fail(err)
		}
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			file.Close()
			if err == nil {
				err = errors.New("select regular files for ZMODEM upload")
			}
			return fail(err)
		}
		if info.Size() < 0 || info.Size() > math.MaxUint32 {
			file.Close()
			return fail(errors.New("ZMODEM upload exceeds the 32-bit file size limit"))
		}
		offer := FileOffer{Name: filepath.Base(path), Size: info.Size(), SizeKnown: true, Modified: info.ModTime()}
		if _, err = parseOffer([]byte(fmt.Sprintf("%s\x00%d\x00", offer.Name, offer.Size))); err != nil {
			file.Close()
			return fail(err)
		}
		files = append(files, Upload{Offer: offer, Reader: file})
	}
	if err := ctx.Err(); err != nil {
		return fail(context.Cause(ctx))
	}
	return files, nil
}
