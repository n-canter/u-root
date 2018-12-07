// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multiboot

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
)

const (
	TrampolineEBX = "u-root-ebx-long"
	TrampolineEP  = "u-root-ep-quad"
)

func (m *Multiboot) addTrampoline() error {
	// Trampoline setups the machine registers to desired state
	// and executes the loaded kernel.
	d, err := setupTrampoline(m.trampoline, m.InfoAddr, m.KernelEntry)
	if err != nil {
		return err
	}

	addr, err := m.mem.AddKexecSegment(d)
	if err != nil {
		return err
	}

	m.EntryPoint = addr
	return nil
}

func setupTrampoline(path string, infoAddr, entryPoint uintptr) ([]byte, error) {
	trampoline, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read trampoline file: %v", err)
	}

	replace := func(d, label, buf []byte) error {
		ind := bytes.Index(d, label)
		if ind == -1 {
			return fmt.Errorf("%q label not found in file", label)
		}
		if len(d) < ind+len(label)+len(buf) {
			return io.ErrUnexpectedEOF
		}
		copy(d[ind+len(label):], buf)
		return nil
	}

	buf := make([]byte, 4+8)
	nativeEndian.PutUint32(buf, uint32(infoAddr))
	nativeEndian.PutUint64(buf[4:], uint64(entryPoint))
	// Patch the trampoline code to store value for ebx register
	// right after "u-root-ebx-long" byte sequence and value
	// for kernel entry point, right after "u-root-ep-quad" byte sequence.
	if err := replace(trampoline, []byte(TrampolineEBX), buf[:4]); err != nil {
		return nil, err
	}
	if err := replace(trampoline, []byte(TrampolineEP), buf[4:]); err != nil {
		return nil, err
	}
	return trampoline, nil
}
