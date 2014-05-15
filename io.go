package fslm

import (
	"encoding/gob"
	"github.com/kho/easy"
	"github.com/kho/stream"
	"io"
)

func FromGob(in io.Reader) (*Hashed, error) {
	var m Hashed
	if err := gob.NewDecoder(in).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func FromGobFile(path string) (*Hashed, error) {
	in, err := easy.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return FromGob(in)
}

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
