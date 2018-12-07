// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kexec

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"
)

func TestParseMemoryMap(t *testing.T) {
	var mem Memory
	root, err := ioutil.TempDir("", "memmap")
	if err != nil {
		t.Fatalf("Cannot create test dir: %v", err)
	}
	defer os.RemoveAll(root)

	old := memoryMapRoot
	memoryMapRoot = root
	defer func() { memoryMapRoot = old }()

	var want []PhysicalMemory
	for i := 0; i < 4; i++ {
		p := path.Join(root, fmt.Sprint(i))
		if err := os.Mkdir(p, 0755); err != nil {
			t.Fatalf("Cannot create test dir: %v", err)
		}

		var typ RangeType
		switch i {
		case 0:
			typ = RangeRAM
		case 1:
			typ = RangeNVACPI
		case 2:
			typ = RangeACPI
		case 3:
			typ = RangeNVS
		}

		start := uintptr(i * 100)
		size := uint(50)
		end := start + uintptr(size)
		if err := ioutil.WriteFile(path.Join(p, "start"), []byte(fmt.Sprintf("%#x\n", start)), 0655); err != nil {
			t.Fatalf("Cannot write test file: %v", err)
		}
		if err := ioutil.WriteFile(path.Join(p, "end"), []byte(fmt.Sprintf("%#x\n", end)), 0655); err != nil {
			t.Fatalf("Cannot write test file: %v", err)
		}
		if err := ioutil.WriteFile(path.Join(p, "type"), append([]byte(typ), '\n'), 0655); err != nil {
			t.Fatalf("Cannot write test file: %v", err)
		}
		want = append(want, PhysicalMemory{
			Range: Range{
				Start: start,
				Size:  size,
			},
			Type: typ,
		})
	}
	if err := mem.ParseMemoryMap(); err != nil {
		t.Fatalf("ParseMemoryMap() error: %v", err)
	}
	if !reflect.DeepEqual(mem.Phys, want) {
		t.Errorf("ParseMemoryMap() got %v, want %v", mem.Phys, want)
	}
}

func TestMemorySub(t *testing.T) {
	var mem Memory
	mem.Phys = []PhysicalMemory{
		PhysicalMemory{Range: Range{Start: 0, Size: 100}, Type: RangeRAM},
		PhysicalMemory{Range: Range{Start: 200, Size: 100}, Type: RangeRAM},
		PhysicalMemory{Range: Range{Start: 400, Size: 100}, Type: RangeRAM},
		PhysicalMemory{Range: Range{Start: 600, Size: 100}, Type: RangeRAM},
	}

	mem.Segments = []Segment{
		Segment{Phys: Range{Start: 0, Size: 50}},
		Segment{Phys: Range{Start: 100, Size: 100}},
		Segment{Phys: Range{Start: 250, Size: 50}},
		Segment{Phys: Range{Start: 410, Size: 80}},
		Segment{Phys: Range{Start: 599, Size: 102}},
	}

	want := []PhysicalMemory{
		PhysicalMemory{Range: Range{Start: 50, Size: 50}, Type: RangeRAM},
		PhysicalMemory{Range: Range{Start: 200, Size: 50}, Type: RangeRAM},
		PhysicalMemory{Range: Range{Start: 400, Size: 10}, Type: RangeRAM},
		PhysicalMemory{Range: Range{Start: 490, Size: 10}, Type: RangeRAM},
	}

	got := mem.sub()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sub() got %+v, want %+v", got, want)
	}
}
