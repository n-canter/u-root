// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Module defines modules to be loaded along with the kernel.
// https://www.gnu.org/software/grub/manual/multiboot/multiboot.html#Boot-information-format.

package multiboot

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
)

var errEmptyModule = errors.New("empty module")

// A Module represents a module to be loaded along with the kernel.
type Module struct {
	// Start is the inclusive start of the Module memory location
	Start uint32
	// End is the exclusive end of the Module memory location.
	End uint32

	// CmdLine is a pointer to a zere-terminated ASCII string.
	CmdLine uint32

	// Reserved is always zero.
	Reserved uint32
}

type module struct {
	start   uint32
	end     uint32
	cmdLine string
}

type modules []module

var sizeofModule = binary.Size(Module{})

func (m modules) size() (uint, error) {
	b, err := m.marshal(0)
	return uint(len(b)), err
}

// marshal writes out the exact bytes of modules to be loaded
// along with the kerne.
func (m modules) marshal(base uintptr) ([]byte, error) {
	cmdBuf := bytes.Buffer{}
	dataBuf := bytes.Buffer{}
	for _, mod := range m {
		offset := uint32(base) + uint32(cmdBuf.Len())
		err := binary.Write(&dataBuf, nativeEndian, Module{
			Start:   mod.start,
			End:     mod.end,
			CmdLine: offset,
		})

		if err != nil {
			return nil, err
		}

		if _, err := cmdBuf.WriteString(mod.cmdLine); err != nil {
			return nil, err
		}

		if err := cmdBuf.WriteByte(0); err != nil {
			return nil, err
		}
	}
	data := append(cmdBuf.Bytes(), dataBuf.Bytes()...)
	return data, nil
}

func (m *Multiboot) addModules() (uintptr, error) {
	var modules modules
	for _, mod := range m.modules {
		args := strings.Fields(mod)
		m, err := m.addModule(args[0], args[1:]...)
		if err == errEmptyModule {
			continue
		}
		if err != nil {
			return 0, err
		}
		modules = append(modules, *m)
	}

	size, err := modules.size()
	if err != nil {
		return 0, err
	}
	addr, err := m.mem.FindSpace(size)
	if err != nil {
		return 0, err
	}

	d, err := modules.marshal(addr)
	if err != nil {
		return 0, err
	}

	addr, err = m.mem.AddKexecSegment(d)
	if err != nil {
		return 0, err
	}
	return addr, nil
}

func (m *Multiboot) addModule(name string, args ...string) (*module, error) {
	data, err := readModule(name)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errEmptyModule
	}
	addr, err := m.mem.AddKexecSegment(data)
	if err != nil {
		return nil, err
	}
	return &module{
		start:   uint32(addr),
		end:     uint32(addr) + uint32(len(data)) - 1,
		cmdLine: strings.Join(args, " "),
	}, nil
}

func readGzip(r io.Reader) ([]byte, error) {
	z, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	return ioutil.ReadAll(z)
}

func readModule(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := readGzip(f)
	if err == nil {
		return b, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("cannot rewind file: %v", err)
	}

	return ioutil.ReadAll(f)
}
