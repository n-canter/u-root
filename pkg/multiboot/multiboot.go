// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package multiboot implements basic primitives
// to load multiboot kernels as defined in
// https://www.gnu.org/software/grub/manual/multiboot/multiboot.html.
package multiboot

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"unsafe"

	"github.com/u-root/u-root/pkg/kexec"
)

var nativeEndian binary.ByteOrder

func init() {
	var i = 0x0100
	ptr := unsafe.Pointer(&i)
	if 0x01 == *(*byte)(ptr) {
		nativeEndian = binary.BigEndian
	} else if 0x00 == *(*byte)(ptr) {
		nativeEndian = binary.LittleEndian
	}
}

const bootloader = "u-root kexec"

// Multiboot defines parameters for working with multiboot kernels.
type Multiboot struct {
	mem kexec.Memory

	file    string
	modules []string

	cmdLine    string
	bootloader string

	// trampoline is path to trampoline path to executable blob, which should set machine
	// to a specific state defined by multiboot v1 spec (will be removed in future releases).
	// https://www.gnu.org/software/grub/manual/multiboot/multiboot.html#Machine-state.

	// Trampoline should use a long word value stored right after "u-root-ebx-long" byte sequence
	// as a value to be stored in ebx register and use a quad word value stored right after
	// "u-root-ep-quad" as kernel entry point.
	trampoline string

	header Header

	// InfoAddr is a pointer to multiboot info in one of the Segments.
	InfoAddr uintptr
	// EntryPoint is a pointer to entry point.
	EntryPoint uintptr
	// KernelEntry is a pointer to entry point of kernel.
	KernelEntry uintptr
}

var rangeTypes = map[kexec.RangeType]uint32{
	kexec.RangeRAM:     1,
	kexec.RangeDefault: 2,
	kexec.RangeNVACPI:  3,
	kexec.RangeACPI:    3,
	kexec.RangeNVS:     4,
}

var sizeofMemoryMap = uint(binary.Size(MemoryMap{}))

// MemoryMap represents a reserved range of memory passed via the Multiboot Info header.
type MemoryMap struct {
	// Size is the size of the associated structure in bytes.
	Size uint32
	// BaseAddr is the starting address.
	BaseAddr uint64
	// Length is the size of the memory region in bytes.
	Length uint64
	// Type is the variety of address range represented.
	Type uint32
}

type memoryMaps []MemoryMap

// Probe checks if file is multiboot v1 kernel.
func Probe(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = parseHeader(f)
	return err
}

// New returns a new Multiboot instance.
func New(file, cmdLine, trampoline string, modules []string) *Multiboot {
	return &Multiboot{
		file:       file,
		modules:    modules,
		cmdLine:    cmdLine,
		trampoline: trampoline,
		bootloader: bootloader,
		mem:        kexec.Memory{},
	}
}

// Load loads and parses multiboot information from m.file.
func (m *Multiboot) Load() error {
	log.Printf("Parsing file %v", m.file)
	kernel, err := os.Open(m.file)
	if err != nil {
		return err
	}
	defer kernel.Close()

	log.Printf("Getting kernel entry point")
	if m.KernelEntry, err = getEntryPoint(kernel); err != nil {
		return fmt.Errorf("Error getting kernel entry point: %v", err)
	}

	if _, err := kernel.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("Error seeking to the start of file: %v", err)
	}

	log.Println("Parsing Multiboot Header")
	if m.header, err = parseHeader(kernel); err != nil {
		return fmt.Errorf("Error parsing headers: %v", err)
	}

	log.Printf("Parsing ELF segments")
	if err := m.mem.LoadElfSegments(kernel); err != nil {
		return fmt.Errorf("Error loading ELF segments: %v", err)
	}

	log.Printf("Parsing memory map")
	if err := m.mem.ParseMemoryMap(); err != nil {
		return fmt.Errorf("Error parsing memory map: %v", err)
	}

	log.Printf("Preparing Multiboot Info")
	if m.InfoAddr, err = m.addInfo(); err != nil {
		return fmt.Errorf("Error preparing Multiboot Info: %v", err)
	}
	log.Printf("Adding trampoline")
	if err := m.addTrampoline(); err != nil {
		return fmt.Errorf("Error adding trampoline: %v", err)
	}
	return nil
}

func getEntryPoint(r io.ReaderAt) (uintptr, error) {
	f, err := elf.NewFile(r)
	if err != nil {
		return 0, err
	}
	return uintptr(f.Entry), err
}

func (m *Multiboot) addInfo() (addr uintptr, err error) {
	iw, err := m.newMultibootInfo()
	if err != nil {
		return 0, err
	}
	infoSize, err := iw.size()
	if err != nil {
		return 0, err
	}

	addr, err = m.mem.FindSpace(infoSize)
	if err != nil {
		return 0, err
	}

	d, err := iw.marshal(addr)
	if err != nil {
		return 0, err
	}

	addr, err = m.mem.AddKexecSegment(d)
	if err != nil {
		return 0, err
	}
	return addr, nil
}

func (m Multiboot) memoryMap() memoryMaps {
	var ret memoryMaps
	for _, r := range m.mem.Phys {
		typ, ok := rangeTypes[r.Type]
		if !ok {
			typ = rangeTypes[kexec.RangeDefault]
		}
		v := MemoryMap{
			// Size is really used for skipping to the next pair.
			Size:     uint32(sizeofMemoryMap) - 4,
			BaseAddr: uint64(r.Start),
			Length:   uint64(r.Size),
			Type:     typ,
		}
		ret = append(ret, v)
	}
	return ret
}

func (m *Multiboot) addMmap() (addr uintptr, size uint, err error) {
	mmap := m.memoryMap()
	d, err := mmap.marshal()
	if err != nil {
		return 0, 0, err
	}
	addr, err = m.mem.AddKexecSegment(d)
	if err != nil {
		return 0, 0, err
	}
	return addr, uint(len(mmap)) * sizeofMemoryMap, nil
}

func (m Multiboot) memoryBoundaries() (lower, upper uint32) {
	const M1 = 1048576
	const K640 = 640 * 1024
	for _, r := range m.mem.Phys {
		if r.Type != kexec.RangeRAM {
			continue
		}
		end := uint32(r.Start) + uint32(r.Size)
		// Lower memory starts at address 0, and upper memory starts at address 1 megabyte.
		// The maximum possible value for lower memory is 640 kilobytes.
		// The value returned for upper memory is maximally the address of the first upper memory hole minus 1 megabyte.
		// It is not guaranteed to be this value.
		if r.Start <= K640 && end > lower {
			lower = end
		}
		if r.Start <= M1 && end > upper+M1 {
			upper = end - M1
		}
	}
	return
}

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func (m *Multiboot) newMultibootInfo() (*infoWrapper, error) {
	mmapAddr, mmapSize, err := m.addMmap()
	if err != nil {
		return nil, err
	}
	var info Info
	if m.header.Flags&flagHeaderMemoryInfo != 0 {
		lower, upper := m.memoryBoundaries()
		info = Info{
			Flags:      flagInfoMemMap | flagInfoMemory,
			MemLower:   min(uint32(lower>>10), 0xFFFFFFFF),
			MemUpper:   min(uint32(upper>>10), 0xFFFFFFFF),
			MmapLength: uint32(mmapSize),
			MmapAddr:   uint32(mmapAddr),
		}
	}

	if len(m.modules) > 0 {
		modAddr, err := m.addModules()
		if err != nil {
			return nil, err
		}
		info.Flags |= flagInfoMods
		info.ModsAddr = uint32(modAddr)
		info.ModsCount = uint32(len(m.modules))
	}

	info.CmdLine = sizeofInfo
	info.BootLoaderName = sizeofInfo + uint32(len(m.cmdLine)) + 1
	info.Flags |= flagInfoCmdLine | flagInfoBootLoaderName

	return &infoWrapper{
		Info:           info,
		CmdLine:        m.cmdLine,
		BootLoaderName: m.bootloader,
	}, nil
}

func (m Multiboot) Segments() []kexec.Segment {
	return m.mem.Segments
}

// marshal writes out the exact bytes expected by the multiboot info header
// specified in
// https://www.gnu.org/software/grub/manual/multiboot/multiboot.html#Boot-information-format.
func (m memoryMaps) marshal() ([]byte, error) {
	buf := bytes.Buffer{}
	err := binary.Write(&buf, nativeEndian, m)
	return buf.Bytes(), err
}
