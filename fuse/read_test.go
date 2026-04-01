// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fuse

import (
	"bytes"
	"os"
	"testing"
)

func TestReadResultMultiFdSize(t *testing.T) {
	r := ReadResultMultiFd([]FdSegment{
		{Fd: 0, Off: 0, Sz: 100},
		{Fd: 0, Off: 0, Sz: 200},
		{Fd: 0, Off: 0, Sz: 50},
	})
	if got := r.Size(); got != 350 {
		t.Errorf("Size() = %d, want 350", got)
	}
}

func TestReadResultMultiFdSizeEmpty(t *testing.T) {
	r := ReadResultMultiFd(nil)
	if got := r.Size(); got != 0 {
		t.Errorf("Size() = %d, want 0", got)
	}
}

func TestReadResultMultiFdBytes(t *testing.T) {
	// Create two temp files with known content
	f1, err := os.CreateTemp(t.TempDir(), "seg1")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := os.CreateTemp(t.TempDir(), "seg2")
	if err != nil {
		t.Fatal(err)
	}

	data1 := bytes.Repeat([]byte("AAAA"), 64) // 256 bytes
	data2 := bytes.Repeat([]byte("BBBB"), 64) // 256 bytes

	if _, err := f1.Write(data1); err != nil {
		t.Fatal(err)
	}
	if _, err := f2.Write(data2); err != nil {
		t.Fatal(err)
	}

	// Read all of file1 + all of file2
	r := ReadResultMultiFd([]FdSegment{
		{Fd: f1.Fd(), Off: 0, Sz: 256},
		{Fd: f2.Fd(), Off: 0, Sz: 256},
	})

	buf := make([]byte, r.Size())
	out, status := r.Bytes(buf)
	if !status.Ok() {
		t.Fatalf("Bytes() returned status %v", status)
	}
	if len(out) != 512 {
		t.Fatalf("Bytes() returned %d bytes, want 512", len(out))
	}
	if !bytes.Equal(out[:256], data1) {
		t.Error("first segment data mismatch")
	}
	if !bytes.Equal(out[256:], data2) {
		t.Error("second segment data mismatch")
	}
}

func TestReadResultMultiFdBytesWithOffset(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "seg")
	if err != nil {
		t.Fatal(err)
	}

	// Write 1024 bytes: 512 of 'X', 512 of 'Y'
	data := append(bytes.Repeat([]byte("X"), 512), bytes.Repeat([]byte("Y"), 512)...)
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}

	// Two segments from the same fd at different offsets
	r := ReadResultMultiFd([]FdSegment{
		{Fd: f.Fd(), Off: 256, Sz: 256}, // latter half of X region
		{Fd: f.Fd(), Off: 512, Sz: 128}, // first part of Y region
	})

	buf := make([]byte, r.Size())
	out, status := r.Bytes(buf)
	if !status.Ok() {
		t.Fatalf("Bytes() returned status %v", status)
	}
	if len(out) != 384 {
		t.Fatalf("got %d bytes, want 384", len(out))
	}

	// First 256 bytes should be 'X'
	for i := 0; i < 256; i++ {
		if out[i] != 'X' {
			t.Errorf("byte %d = %q, want 'X'", i, out[i])
			break
		}
	}
	// Next 128 bytes should be 'Y'
	for i := 256; i < 384; i++ {
		if out[i] != 'Y' {
			t.Errorf("byte %d = %q, want 'Y'", i, out[i])
			break
		}
	}
}

func TestReadResultMultiFdBytesShortBuffer(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "seg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(bytes.Repeat([]byte("Z"), 1024)); err != nil {
		t.Fatal(err)
	}

	r := ReadResultMultiFd([]FdSegment{
		{Fd: f.Fd(), Off: 0, Sz: 512},
		{Fd: f.Fd(), Off: 512, Sz: 512},
	})

	// Provide a buffer smaller than total size — should only fill what fits
	buf := make([]byte, 600)
	out, status := r.Bytes(buf)
	if !status.Ok() {
		t.Fatalf("Bytes() returned status %v", status)
	}
	if len(out) != 600 {
		t.Fatalf("got %d bytes, want 600", len(out))
	}
	for i, b := range out {
		if b != 'Z' {
			t.Errorf("byte %d = %q, want 'Z'", i, b)
			break
		}
	}
}

func TestReadResultMultiFdBytesShortRead(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "seg")
	if err != nil {
		t.Fatal(err)
	}
	// File only has 100 bytes but segment asks for 200
	if _, err := f.Write(bytes.Repeat([]byte("Q"), 100)); err != nil {
		t.Fatal(err)
	}

	r := ReadResultMultiFd([]FdSegment{
		{Fd: f.Fd(), Off: 0, Sz: 200},
	})

	buf := make([]byte, 200)
	out, status := r.Bytes(buf)
	if !status.Ok() {
		t.Fatalf("Bytes() returned status %v", status)
	}
	// Should get only 100 bytes (short read at EOF)
	if len(out) != 100 {
		t.Fatalf("got %d bytes, want 100", len(out))
	}
}

func TestReadResultMultiFdDone(t *testing.T) {
	// Done() should not panic
	r := ReadResultMultiFd([]FdSegment{})
	r.Done()
}
