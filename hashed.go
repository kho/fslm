package fslm

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/kho/word"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

// Hashed is a finite-state representation of a n-gram language model
// using hash tables. A Hashed model is usually loaded from file or
// constructed with a Builder.
type Hashed struct {
	// The vocabulary of the model. Don't modify this. If you need to
	// have a vocab based on this, make a copy using Vocab.Copy().
	vocab *word.Vocab
	// Sentence boundary symbols.
	bos, eos     string
	bosId, eosId word.Id
	// Buckets per state for out-going lexical transitions.
	// There are three kinds of transitions:
	//
	// (1) A lexical transition that consumes an actual word (i.e. any
	// valid word other than <s> or </s>). This leads to a valid state
	// with some weight. Note we allow transition from empty consuming
	// <s>. This transition should have WEIGHT_LOG0 anyway (e.g. those
	// built from SRILM) so keeping it doesn't cause much trouble.
	//
	// (2) A final transition that consumes </s>. This gives the final
	// weight but always leads to an invalid state.
	//
	// (3) Buckets with invalid keys (word.NIL) are all filled with
	// back-off transitions so that we know the back-off transition
	// immediately when the key cannot be found.
	transitions []xqwBuckets
}

func (m *Hashed) Start() StateId {
	return _STATE_START
}

func (m *Hashed) NextI(p StateId, i word.Id) (q StateId, w Weight) {
	// Try backing off until we find the n-gram or hit empty state.
	next := m.transitions[p].FindEntry(i)
	for next.Key == word.NIL && p != _STATE_EMPTY {
		p = next.Value.State
		w += next.Value.Weight
		next = m.transitions[p].FindEntry(i)
	}
	if next.Key != word.NIL {
		q = next.Value.State
		w += next.Value.Weight
	} else {
		q = _STATE_EMPTY
		w = WEIGHT_LOG0
	}
	return
}

func (m *Hashed) NextS(p StateId, s string) (q StateId, w Weight) {
	return m.NextI(p, m.vocab.IdOf(s))
}

func (m *Hashed) Final(p StateId) Weight {
	_, w := m.NextI(p, m.eosId)
	return w
}

func (m *Hashed) BackOff(p StateId) (StateId, Weight) {
	if p == _STATE_EMPTY {
		return STATE_NIL, 0
	}
	backoff := m.transitions[p].FindEntry(word.NIL).Value
	return backoff.State, backoff.Weight
}

func (m *Hashed) Vocab() (*word.Vocab, string, string, word.Id, word.Id) {
	return m.vocab, m.bos, m.eos, m.bosId, m.eosId
}

func (m *Hashed) NumStates() int {
	return len(m.transitions)
}

func (m *Hashed) Transitions(p StateId) chan WordStateWeight {
	ch := make(chan WordStateWeight)
	go func() {
		for i := range m.transitions[p].Range() {
			ch <- WordStateWeight{i.Key, i.Value.State, i.Value.Weight}
		}
		close(ch)
	}()
	return ch
}

// MarshalBinary uses gob, which is unfortunately very slow even for a
// modestly sized model.
func (m *Hashed) MarshalBinary() (data []byte, err error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err = enc.Encode(m.vocab); err != nil {
		return
	}
	if err = enc.Encode(m.bos); err != nil {
		return
	}
	if err = enc.Encode(m.eos); err != nil {
		return
	}
	if err = enc.Encode(m.transitions); err != nil {
		return
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary uses gob, which is unfortunately very slow even for
// a modestly sized model.
func (m *Hashed) UnmarshalBinary(data []byte) (err error) {
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err = dec.Decode(&m.vocab); err != nil {
		return
	}
	if err = dec.Decode(&m.bos); err != nil {
		return
	}
	if err = dec.Decode(&m.eos); err != nil {
		return
	}
	if err = dec.Decode(&m.transitions); err != nil {
		return
	}
	if m.bosId = m.vocab.IdOf(m.bos); m.bosId == word.NIL {
		return errors.New(m.bos + " not in vocabulary")
	}
	if m.eosId = m.vocab.IdOf(m.eos); m.eosId == word.NIL {
		return errors.New(m.eos + " not in vocabulary")
	}
	return nil
}

const hashedMagic = "#fslm.hash"

func (m *Hashed) header() (header []byte, err error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err = enc.Encode(m.vocab); err != nil {
		return
	}
	if err = enc.Encode(m.bos); err != nil {
		return
	}
	if err = enc.Encode(m.eos); err != nil {
		return
	}
	numBuckets := make([]int, len(m.transitions))
	for i, t := range m.transitions {
		numBuckets[i] = len(t)
	}
	if err = enc.Encode(numBuckets); err != nil {
		return
	}
	header = buf.Bytes()
	return
}

func (m *Hashed) parseHeader(header []byte) (numBuckets []int, err error) {
	dec := gob.NewDecoder(bytes.NewReader(header))
	if err = dec.Decode(&m.vocab); err != nil {
		return
	}
	if err = dec.Decode(&m.bos); err != nil {
		return
	}
	if err = dec.Decode(&m.eos); err != nil {
		return
	}
	if m.bosId = m.vocab.IdOf(m.bos); m.bosId == word.NIL {
		err = errors.New(m.bos + " not in vocabulary")
		return
	}
	if m.eosId = m.vocab.IdOf(m.eos); m.eosId == word.NIL {
		err = errors.New(m.eos + " not in vocabulary")
		return
	}
	if err = dec.Decode(&numBuckets); err != nil {
		return
	}
	return
}

func (m *Hashed) WriteBinary(path string) (err error) {
	w, err := os.Create(path)
	if err != nil {
		return
	}
	defer w.Close()
	if _, err = w.Write([]byte(hashedMagic)); err != nil {
		return
	}
	// Header: binary.MaxVarintLen64 bytes of header length and then
	// header.
	header, err := m.header()
	if err != nil {
		return
	}
	headerLenBytes := make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(headerLenBytes, uint64(len(header)))
	if _, err = w.Write(headerLenBytes); err != nil {
		return
	}
	if _, err = w.Write(header); err != nil {
		return
	}
	// Raw entries.
	written, err := w.Seek(0, 1)
	if err != nil {
		return
	}
	// Paddings so that each entry is properly aligned.
	align := unsafe.Alignof(xqwEntry{})
	if _, err = w.Write(make([]byte, align-uintptr(written)%align)); err != nil {
		return
	}
	// Actual entries.
	size := unsafe.Sizeof(xqwEntry{})
	for _, i := range m.transitions {
		iHeader := (*reflect.SliceHeader)(unsafe.Pointer(&i))
		var bytes []byte
		bytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
		bytesHeader.Data = iHeader.Data
		bytesHeader.Len = int(uintptr(iHeader.Len) * size)
		bytesHeader.Cap = bytesHeader.Len
		if _, err = w.Write(bytes); err != nil {
			return
		}
	}
	return nil
}

func (m *Hashed) unsafeParseBinary(raw []byte) error {
	if string(raw[:len(hashedMagic)]) != hashedMagic {
		return errors.New("not a FSLM binary file")
	}
	read := uintptr(len(hashedMagic))
	headerLen, varintErr := binary.Uvarint(raw[read : read+binary.MaxVarintLen64])
	if varintErr <= 0 {
		return errors.New("error reading header size")
	}
	read += binary.MaxVarintLen64
	numBuckets, err := m.parseHeader(raw[read : read+uintptr(headerLen)])
	if err != nil {
		return err
	}
	read += uintptr(headerLen)
	align, size := unsafe.Alignof(xqwEntry{}), unsafe.Sizeof(xqwEntry{})
	read += align - read%align
	// The rest are actual entries.
	if (uintptr(len(raw))-read)%size != 0 {
		return errors.New(fmt.Sprintf("number of left-over bytes are not a multiple of %d", size))
	}
	entryBytes := raw[read:]
	var entrySlice []xqwEntry
	entryBytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&entryBytes))
	entrySliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&entrySlice))
	entrySliceHeader.Data = entryBytesHeader.Data
	entrySliceHeader.Len = entryBytesHeader.Len / int(size)
	entrySliceHeader.Cap = entrySliceHeader.Len
	m.transitions = make([]xqwBuckets, len(numBuckets))
	low := 0
	for i, n := range numBuckets {
		m.transitions[i] = xqwBuckets(entrySlice[low : low+n])
		low += n
	}
	return nil
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

func FromBinary(path string) (*Hashed, *MappedFile, error) {
	m, err := OpenMappedFile(path)
	if err != nil {
		return nil, nil, err
	}
	var model Hashed
	if err := model.unsafeParseBinary(m.data); err != nil {
		return nil, nil, err
	}
	return &model, m, nil
}
