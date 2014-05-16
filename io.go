package fslm

import (
	"errors"
	"fmt"
	"github.com/kho/easy"
	"github.com/kho/stream"
	"io"
	"os"
	"syscall"
)

func FromARPA(in io.Reader, scale float64) (*Hashed, error) {
	builder := NewBuilder(nil, "", "")
	if err := stream.Run(stream.EnumRead(in, lineSplit), arpaTop(builder)); err != nil {
		return nil, err
	}
	return builder.DumpHashed(scale), nil
}

func FromARPAFile(path string, scale float64) (*Hashed, error) {
	in, err := easy.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return FromARPA(in, scale)
}

type MappedFile struct {
	file *os.File
	data []byte
}

func OpenMappedFile(path string) (m *MappedFile, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	stat, err := f.Stat()
	if err != nil {
		return
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, int(stat.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return
	}
	m = &MappedFile{f, data}
	return
}

func (m *MappedFile) Close() error {
	err1 := syscall.Munmap(m.data)
	err2 := m.file.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func FromBinary(path string) (int, interface{}, *MappedFile, error) {
	m, err := OpenMappedFile(path)
	if err != nil {
		return -1, nil, nil, err
	}
	if len(m.data) < MAGIC_BYTES || string(m.data[:6]) != "#fslm." {
		return -1, nil, nil, errors.New("not a FSLM binary file")
	}
	switch string(m.data[:MAGIC_BYTES]) {
	case MAGIC_HASHED:
		var model Hashed
		if err := model.UnsafeParseBinary(m.data); err != nil {
			return -1, nil, nil, err
		}
		return MODEL_HASHED, &model, m, nil
	case MAGIC_SORTED:
		var model Sorted
		if err := model.UnsafeParseBinary(m.data); err != nil {
			return -1, nil, nil, err
		}
		return MODEL_SORTED, &model, m, nil
	default:
		return -1, nil, nil, errors.New(fmt.Sprintf("unknown FSLM format: %q", m.data[6:MAGIC_BYTES]))
	}
}
