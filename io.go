package fslm

import (
	"encoding/gob"
	"github.com/kho/easy"
	"github.com/kho/stream"
	"io"
)

func FromGob(in io.Reader) (*Model, error) {
	var m Model
	if err := gob.NewDecoder(in).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func FromGobFile(path string) (*Model, error) {
	in, err := easy.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return FromGob(in)
}

func FromARPA(in io.Reader) (*Model, error) {
	builder := NewBuilder(nil)
	if err := stream.Run(stream.EnumRead(in, lineSplit), arpaTop{builder}); err != nil {
		return nil, err
	}
	return builder.Dump(), nil
}

func FromARPAFile(path string) (*Model, error) {
	in, err := easy.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return FromARPA(in)
}
