// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"io"
	"syscall"
)

// ReadResultData is the read return for returning bytes directly.
type readResultData struct {
	// Raw bytes for the read.
	Data []byte
}

func (r *readResultData) Size() int {
	return len(r.Data)
}

func (r *readResultData) Done() {
}

func (r *readResultData) Bytes(buf []byte) ([]byte, Status) {
	return r.Data, OK
}

func ReadResultData(b []byte) ReadResult {
	return &readResultData{b}
}

func ReadResultFd(fd uintptr, off int64, sz int) ReadResult {
	return &readResultFd{fd, off, sz}
}

// ReadResultFd is the read return for zero-copy file data.
type readResultFd struct {
	// Splice from the following file.
	Fd uintptr

	// Offset within Fd, or -1 to use current offset.
	Off int64

	// Size of data to be loaded. Actual data available may be
	// less at the EOF.
	Sz int
}

// Reads raw bytes from file descriptor if necessary, using the passed
// buffer as storage.
func (r *readResultFd) Bytes(buf []byte) ([]byte, Status) {
	sz := r.Sz
	if len(buf) < sz {
		sz = len(buf)
	}

	n, err := syscall.Pread(int(r.Fd), buf[:sz], r.Off)
	if err == io.EOF {
		err = nil
	}

	if n < 0 {
		n = 0
	}

	return buf[:n], ToStatus(err)
}

func (r *readResultFd) Size() int {
	return r.Sz
}

func (r *readResultFd) Done() {
}

// FdSegment describes one contiguous region to read from a file descriptor.
type FdSegment struct {
	Fd  uintptr
	Off int64
	Sz  int
}

// readResultMultiFd is a ReadResult backed by multiple fd+offset+size segments.
type readResultMultiFd struct {
	Segments []FdSegment
	totalSz  int
}

// ReadResultMultiFd creates a ReadResult that splices data from multiple
// file descriptors (or multiple regions of the same fd) in order.
// The caller is responsible for closing the fds after Done() is called.
func ReadResultMultiFd(segments []FdSegment) ReadResult {
	total := 0
	for _, s := range segments {
		total += s.Sz
	}
	return &readResultMultiFd{Segments: segments, totalSz: total}
}

func (r *readResultMultiFd) Size() int {
	return r.totalSz
}

func (r *readResultMultiFd) Done() {
}

func (r *readResultMultiFd) Bytes(buf []byte) ([]byte, Status) {
	pos := 0
	for _, seg := range r.Segments {
		sz := seg.Sz
		if pos+sz > len(buf) {
			sz = len(buf) - pos
		}
		if sz <= 0 {
			break
		}
		n, err := syscall.Pread(int(seg.Fd), buf[pos:pos+sz], seg.Off)
		if err == io.EOF {
			err = nil
		}
		if n < 0 {
			n = 0
		}
		if err != nil {
			return buf[:pos+n], ToStatus(err)
		}
		pos += n
		if n < sz {
			break // short read (EOF within segment)
		}
	}
	return buf[:pos], OK
}
