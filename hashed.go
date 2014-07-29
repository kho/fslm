package fslm

import (
	"bytes"
	"encoding/gob"
	"errors"
	"os"
	"reflect"
	"unsafe"

	"github.com/kho/byteblock"
	"github.com/kho/word"
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
	bw := byteblock.NewByteBlockWriter(w)
	if err = bw.WriteString(MAGIC_HASHED, 0); err != nil {
		return
	}
	// Header
	header, err := m.header()
	if err != nil {
		return
	}
	if err = bw.Write(header, 0); err != nil {
		return
	}
	// Raw entries.

	// Go over the transitions to see how many entries there are in total.
	numEntries := int64(0)
	for _, i := range m.transitions {
		numEntries += int64(len(i))
	}
	// Ask for a large new block and then incrementally write out the
	// data.
	align := int64(unsafe.Alignof(xqwEntry{}))
	size := int64(unsafe.Sizeof(xqwEntry{}))
	if err = bw.NewBlock(align, size*numEntries); err != nil {
		return
	}
	for _, i := range m.transitions {
		iHeader := (*reflect.SliceHeader)(unsafe.Pointer(&i))
		var bytes []byte
		bytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
		bytesHeader.Data = iHeader.Data
		bytesHeader.Len = int(int64(iHeader.Len) * size)
		bytesHeader.Cap = bytesHeader.Len
		if err = bw.Append(bytes); err != nil {
			return
		}
	}
	return nil
}

func IsHashedBinary(raw []byte) bool {
	bs := byteblock.NewByteBlockSlicer(raw)
	magic, err := bs.Slice()
	return err == nil && string(magic) == MAGIC_HASHED
}

func (m *Hashed) UnsafeParseBinary(raw []byte) error {
	bs := byteblock.NewByteBlockSlicer(raw)

	magic, err := bs.Slice()
	if err != nil {
		return err
	}
	if string(magic) != MAGIC_HASHED {
		return errors.New("not a FSLM binary file")
	}

	header, err := bs.Slice()
	if err != nil {
		return err
	}

	numBuckets, err := m.parseHeader(header)
	if err != nil {
		return err
	}

	entryBytes, err := bs.Slice()
	if err != nil {
		return err
	}
	var entrySlice []xqwEntry
	entryBytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&entryBytes))
	entrySliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&entrySlice))
	entrySliceHeader.Data = entryBytesHeader.Data
	entrySliceHeader.Len = entryBytesHeader.Len / int(unsafe.Sizeof(xqwEntry{}))
	entrySliceHeader.Cap = entrySliceHeader.Len
	m.transitions = make([]xqwBuckets, len(numBuckets))
	low := 0
	for i, n := range numBuckets {
		m.transitions[i] = xqwBuckets(entrySlice[low : low+n])
		low += n
	}
	return nil
}
