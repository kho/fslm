package fslm

import (
	"errors"
	"github.com/kho/easy"
	"github.com/kho/stream"
	"io"
	"os"
	"syscall"
)

func FromARPA(in io.Reader) (*Builder, error) {
	builder := NewBuilder(nil, "", "")
	if err := stream.Run(stream.NewScanEnumeratorWith(in, lineSplit), arpaTop(builder)); err != nil {
		return nil, err
	}
	return builder, nil
}

func FromARPAFile(path string) (*Builder, error) {
	in, err := easy.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return FromARPA(in)
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
	if IsHashedBinary(m.data) {
		var model Hashed
		if err := model.UnsafeParseBinary(m.data); err != nil {
			return -1, nil, nil, err
		}
		return MODEL_HASHED, &model, m, nil
	} else if IsSortedBinary(m.data) {
		var model Sorted
		if err := model.UnsafeParseBinary(m.data); err != nil {
			return -1, nil, nil, err
		}
		return MODEL_SORTED, &model, m, nil
	} else {
		return -1, nil, nil, errors.New("not an FSLM file")
	}
}
