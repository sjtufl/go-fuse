//go:build linux

// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"bytes"
	"os"
	"testing"

	"github.com/hanwen/go-fuse/v2/internal/testutil"
)

// multiFdFS serves a single file "file" whose content is assembled
// from two backing temp files via ReadResultMultiFd.
type multiFdFS struct {
	defaultRawFileSystem
	f1, f2   *os.File
	fileSize uint64
}

func (fs *multiFdFS) Lookup(cancel <-chan struct{}, header *InHeader, name string, out *EntryOut) Status {
	if name != "file" {
		return ENOENT
	}
	*out = EntryOut{
		NodeId: 2,
		Attr: Attr{
			Mode: S_IFREG | 0444,
			Size: fs.fileSize,
		},
		AttrValid: 60,
	}
	return OK
}

func (fs *multiFdFS) Open(cancel <-chan struct{}, input *OpenIn, out *OpenOut) Status {
	if input.NodeId != 2 {
		return ENOENT
	}
	return OK
}

func (fs *multiFdFS) Read(cancel <-chan struct{}, input *ReadIn, buf []byte) (ReadResult, Status) {
	if input.NodeId != 2 {
		return nil, ENOENT
	}

	off := int64(input.Offset)
	sz := int64(input.Size)
	if off >= int64(fs.fileSize) {
		return ReadResultData(nil), OK
	}
	if off+sz > int64(fs.fileSize) {
		sz = int64(fs.fileSize) - off
	}

	// The logical file is [f1 content | f2 content].
	// Each backing file is fileSize/2 bytes long.
	half := int64(fs.fileSize / 2)
	var segments []FdSegment

	pos := off
	end := off + sz
	// Portion from f1
	if pos < half {
		n := half - pos
		if pos+n > end {
			n = end - pos
		}
		segments = append(segments, FdSegment{
			Fd: f1Fd(fs), Off: pos, Sz: int(n),
		})
		pos += n
	}
	// Portion from f2
	if pos < end && pos >= half {
		n := end - pos
		segments = append(segments, FdSegment{
			Fd: f2Fd(fs), Off: pos - half, Sz: int(n),
		})
	}

	return ReadResultMultiFd(segments), OK
}

func f1Fd(fs *multiFdFS) uintptr { return fs.f1.Fd() }
func f2Fd(fs *multiFdFS) uintptr { return fs.f2.Fd() }

func TestMultiFdReadIntegration(t *testing.T) {
	half := 128 * 1024 // 128 KB per backing file
	data1 := bytes.Repeat([]byte("A"), half)
	data2 := bytes.Repeat([]byte("B"), half)

	dir := t.TempDir()
	f1, err := os.CreateTemp(dir, "seg1")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := os.CreateTemp(dir, "seg2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f1.Write(data1); err != nil {
		t.Fatal(err)
	}
	if _, err := f2.Write(data2); err != nil {
		t.Fatal(err)
	}

	mnt := t.TempDir()
	fs := &multiFdFS{f1: f1, f2: f2, fileSize: uint64(2 * half)}
	opts := &MountOptions{
		Debug: testutil.VerboseTest(),
	}
	srv, err := NewServer(fs, mnt, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })
	go srv.Serve()
	if err := srv.WaitMount(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(mnt + "/file")
	if err != nil {
		t.Fatal(err)
	}

	want := append(data1, data2...)
	if !bytes.Equal(got, want) {
		t.Errorf("file content mismatch: got %d bytes, want %d", len(got), len(want))
		// Show first divergence
		for i := range got {
			if i >= len(want) || got[i] != want[i] {
				t.Errorf("first diff at byte %d: got %q, want %q", i, got[i], want[i])
				break
			}
		}
	}
}

// TestMultiFdReadCrossBoundary reads a small region that straddles
// the boundary between the two backing files, exercising the multi-
// segment assembly.
func TestMultiFdReadCrossBoundary(t *testing.T) {
	half := 64 * 1024
	data1 := bytes.Repeat([]byte("X"), half)
	data2 := bytes.Repeat([]byte("Y"), half)

	dir := t.TempDir()
	f1, err := os.CreateTemp(dir, "seg1")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := os.CreateTemp(dir, "seg2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f1.Write(data1); err != nil {
		t.Fatal(err)
	}
	if _, err := f2.Write(data2); err != nil {
		t.Fatal(err)
	}

	mnt := t.TempDir()
	fs := &multiFdFS{f1: f1, f2: f2, fileSize: uint64(2 * half)}
	opts := &MountOptions{
		Debug: testutil.VerboseTest(),
	}
	srv, err := NewServer(fs, mnt, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Unmount() })
	go srv.Serve()
	if err := srv.WaitMount(); err != nil {
		t.Fatal(err)
	}

	// Open the file and pread across the boundary
	fh, err := os.Open(mnt + "/file")
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()

	// Read 8192 bytes starting 4096 bytes before the boundary
	readOff := int64(half - 4096)
	buf := make([]byte, 8192)
	n, err := fh.ReadAt(buf, readOff)
	if err != nil {
		t.Fatal(err)
	}
	if n != 8192 {
		t.Fatalf("ReadAt returned %d bytes, want 8192", n)
	}

	// First 4096 bytes should be 'X', next 4096 should be 'Y'
	for i := 0; i < 4096; i++ {
		if buf[i] != 'X' {
			t.Errorf("byte %d = %q, want 'X'", i, buf[i])
			break
		}
	}
	for i := 4096; i < 8192; i++ {
		if buf[i] != 'Y' {
			t.Errorf("byte %d = %q, want 'Y'", i, buf[i])
			break
		}
	}
}
