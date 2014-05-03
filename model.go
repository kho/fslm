package fslm

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

// Model is a finite-state representation of a n-gram language
// model. A Model is usually loaded from file or constructed with a
// Builder.
type Model struct {
	// The vocabulary of the model. Don't modify this. If you need to
	// have a vocab based on this, make a copy using Vocab.Copy().
	Vocab *Vocab
	// Sentence boundary symbols.
	BOS, EOS     string
	BOSId, EOSId WordId
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
	// (3) Buckets with invalid keys (WORD_NIL) are all filled with
	// back-off transitions so that we know the back-off transition
	// immediately when the key cannot be found.
	transitions []xqwBuckets
}

// Size returns the size of the model in several aspects. This needs
// to iterate over the entire model so it is quite slow. The result
// will not change so if there is need to use them multiple times,
// cache them.
func (m *Model) Size() (numStates, numTransitions, vocabSize int) {
	numStates = len(m.transitions)
	for _, i := range m.transitions {
		numTransitions += i.Size()
	}
	vocabSize = int(m.Vocab.Bound())
	return
}

// Start returns the start state, i.e. the state with context <s>. The
// user should never explicitly query <s>, which has undefined
// behavior (see NextI).
func (m *Model) Start() StateId {
	return _STATE_START
}

// NextI finds out the next state to go from p consuming i. i can not
// be BOSId, EOSId, in which case the result is undefined, but can be
// WORD_NIL. Any i that is not part of m.Vocab is treated as OOV. The
// returned weight w is WEIGHT_LOG0 if and only if unigram i is an OOV
// (note: although rare, it is possible to have "<s> x" but not "x" in
// the LM, in which case "x" is also considered an OOV when not
// occuring as the first token of a sentence).
func (m *Model) NextI(p StateId, i WordId) (q StateId, w Weight) {
	// Try backing off until we find the n-gram or hit empty state.
	next := m.transitions[p].FindEntry(i)
	for next.Key == WORD_NIL && p != _STATE_EMPTY {
		p = next.Value.State
		w += next.Value.Weight
		next = m.transitions[p].FindEntry(i)
	}
	if next.Key != WORD_NIL {
		q = next.Value.State
		w += next.Value.Weight
	} else {
		q = _STATE_EMPTY
		w = WEIGHT_LOG0
	}
	return
}

// NextS is similar to NextI. s can be anything but <s> or </s>, in
// which case the result is undefined.
func (m *Model) NextS(p StateId, s string) (q StateId, w Weight) {
	return m.NextI(p, m.Vocab.IdOf(s))
}

// Final returns the final weight of "consuming" </s> from p. A
// sentence query should finish with this to properly score the
// *whole* sentence.
func (m *Model) Final(p StateId) Weight {
	_, w := m.NextI(p, m.EOSId)
	return w
}

// BackOff returns the back off state and weight of p. The back off
// state of the empty context is STATE_NIL and its weight is
// arbitrary.
func (m *Model) BackOff(p StateId) (StateId, Weight) {
	if p == _STATE_EMPTY {
		return STATE_NIL, 0
	}
	backoff := m.transitions[p].FindEntry(WORD_NIL).Value
	return backoff.State, backoff.Weight
}

// Graphviz prints out the finite-state topology of the model that can
// be visualized with Graphviz. Mostly for debugging; could be quite
// slow.
func (m *Model) Graphviz(w io.Writer) {
	fmt.Fprintln(w, "digraph {")
	fmt.Fprintln(w, "  // lexical transitions")
	for p, es := range m.transitions {
		for e := range es.Range() {
			x, qw := e.Key, e.Value
			fmt.Fprintf(w, "  %d -> %d [label=%q]\n", p, qw.State, fmt.Sprintf("%s : %g", m.Vocab.StringOf(WordId(x)), qw.Weight))
		}
	}
	fmt.Fprintln(w, "  // back-off transitions")
	for p, es := range m.transitions {
		e := es.FindEntry(WORD_NIL)
		fmt.Fprintf(w, "  %d -> %d [label=%q,style=dashed]\n", p, e.Value.State, fmt.Sprintf("%g", e.Value.Weight))
	}
	fmt.Fprintln(w, "}")
}

// TODO: use mmap for disk IO once we have our own hashmap.

// MarshalBinary uses gob, which is unfortunately very slow even for a
// modestly sized model.
func (m *Model) MarshalBinary() (data []byte, err error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err = enc.Encode(m.Vocab); err != nil {
		return
	}
	if err = enc.Encode(m.BOS); err != nil {
		return
	}
	if err = enc.Encode(m.EOS); err != nil {
		return
	}
	if err = enc.Encode(m.transitions); err != nil {
		return
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary uses gob, which is unfortunately very slow even for
// a modestly sized model.
func (m *Model) UnmarshalBinary(data []byte) (err error) {
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err = dec.Decode(&m.Vocab); err != nil {
		return
	}
	if err = dec.Decode(&m.BOS); err != nil {
		return
	}
	if err = dec.Decode(&m.EOS); err != nil {
		return
	}
	if err = dec.Decode(&m.transitions); err != nil {
		return
	}
	if m.BOSId = m.Vocab.IdOf(m.BOS); m.BOSId == WORD_NIL {
		return errors.New(m.BOS + " not in vocabulary")
	}
	if m.EOSId = m.Vocab.IdOf(m.EOS); m.EOSId == WORD_NIL {
		return errors.New(m.EOS + " not in vocabulary")
	}
	return nil
}

const modelMagic = "#fslm.hash"

func (m *Model) header() (header []byte, err error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err = enc.Encode(m.Vocab); err != nil {
		return
	}
	if err = enc.Encode(m.BOS); err != nil {
		return
	}
	if err = enc.Encode(m.EOS); err != nil {
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

func (m *Model) parseHeader(header []byte) (numBuckets []int, err error) {
	dec := gob.NewDecoder(bytes.NewReader(header))
	if err = dec.Decode(&m.Vocab); err != nil {
		return
	}
	if err = dec.Decode(&m.BOS); err != nil {
		return
	}
	if err = dec.Decode(&m.EOS); err != nil {
		return
	}
	if m.BOSId = m.Vocab.IdOf(m.BOS); m.BOSId == WORD_NIL {
		err = errors.New(m.BOS + " not in vocabulary")
		return
	}
	if m.EOSId = m.Vocab.IdOf(m.EOS); m.EOSId == WORD_NIL {
		err = errors.New(m.EOS + " not in vocabulary")
		return
	}
	if err = dec.Decode(&numBuckets); err != nil {
		return
	}
	return
}

func (m *Model) WriteBinary(path string) (err error) {
	w, err := os.Create(path)
	if err != nil {
		return
	}
	defer w.Close()
	if _, err = w.Write([]byte(modelMagic)); err != nil {
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

func (m *Model) unsafeParseBinary(raw []byte) error {
	if string(raw[:len(modelMagic)]) != modelMagic {
		return errors.New("not a FSLM binary file")
	}
	read := uintptr(len(modelMagic))
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

func FromBinary(path string) (*Model, *MappedFile, error) {
	m, err := OpenMappedFile(path)
	if err != nil {
		return nil, nil, err
	}
	var model Model
	if err := model.unsafeParseBinary(m.data); err != nil {
		return nil, nil, err
	}
	return &model, m, nil
}
