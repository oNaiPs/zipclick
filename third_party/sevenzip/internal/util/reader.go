// Package util implements various utility types and interfaces.
package util

import (
	"bufio"
	"io"
)

// SizeReadSeekCloser is an io.Reader, io.Seeker, and io.Closer with a Size
// method.
type SizeReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
	Size() int64
}

// Reader is both an io.Reader and io.ByteReader.
type Reader interface {
	io.Reader
	io.ByteReader
}

// ReadCloser is a Reader that is also an io.Closer.
type ReadCloser interface {
	Reader
	io.Closer
}

type nopCloser struct {
	Reader
}

func (nopCloser) Close() error {
	return nil
}

// NopCloser returns a ReadCloser with a no-op Close method wrapping the
// provided Reader r.
func NopCloser(r Reader) ReadCloser {
	return &nopCloser{r}
}

// ZipClick patch: buffered. Every caller reads its stream through to the
// end as decoder input, and an unbuffered ReadByte cost a full Read down
// the decoder chain per byte: 78% of the time on a BCJ2 7z.
type byteReadCloser struct {
	*bufio.Reader
	c io.Closer
}

func (rc *byteReadCloser) Close() error { return rc.c.Close() } //nolint:wrapcheck

// ByteReadCloser returns a ReadCloser either by returning the io.ReadCloser
// r if it implements the interface, or wrapping it with a ReadByte method.
func ByteReadCloser(r io.ReadCloser) ReadCloser {
	if rc, ok := r.(ReadCloser); ok {
		return rc
	}

	return &byteReadCloser{bufio.NewReaderSize(r, 64<<10), r}
}
