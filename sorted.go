package fslm

import (
	"bytes"
	"encoding/gob"
	"errors"
	"github.com/kho/byteblock"
	"github.com/kho/word"
	"os"
	"reflect"
	"unsafe"
)

type Sorted struct {
	// The vocabulary of the model. Don't modify this. If you need to
	// have a vocab based on this, make a copy using Vocab.Copy().
	vocab *word.Vocab
	// Sentence boundary symbols.
	bos, eos     string
	bosId, eosId word.Id
	// Transitions indexed by state and sorted by label. Back-off
	// transitions are stored as transitions consuming word.NIL.
	transitions [][]WordStateWeight
}

func (m *Sorted) Start() StateId {
	return _STATE_START
}

func (m *Sorted) NextI(p StateId, x word.Id) (q StateId, w Weight) {
	next := m.findNext(p, x)
	for next.Word == word.NIL && p != _STATE_EMPTY {
		p = next.State
		w += next.Weight
		next = m.findNext(p, x)
	}
	if next.Word != word.NIL {
		q = next.State
		w += next.Weight
	} else {
		q = _STATE_EMPTY
		w = WEIGHT_LOG0
	}
	return
}

func (m *Sorted) findNext(p StateId, x word.Id) *WordStateWeight {
	next := m.transitions[p]
	// Search for x using binary search.
	l, h := 0, len(next)
	for l < h {
		mid := l + (h-l)>>1
		xMid := next[mid].Word
		if xMid < x {
			l = mid + 1
		} else if xMid > x {
			h = mid
		} else {
			return &next[mid]
		}
	}
	// Not found, take the back-off transitions.
	return &next[len(next)-1]
}

func (m *Sorted) NextS(p StateId, s string) (q StateId, w Weight) {
	return m.NextI(p, m.vocab.IdOf(s))
}

func (m *Sorted) Final(p StateId) Weight {
	_, w := m.NextI(p, m.eosId)
	return w
}

func (m *Sorted) BackOff(p StateId) (StateId, Weight) {
	if p == _STATE_EMPTY {
		return STATE_NIL, 0
	}
	next := m.transitions[p]
	backoff := next[len(next)-1]
	return backoff.State, backoff.Weight
}

func (m *Sorted) Vocab() (*word.Vocab, string, string, word.Id, word.Id) {
	return m.vocab, m.bos, m.eos, m.bosId, m.eosId
}

func (m *Sorted) NumStates() int {
	return len(m.transitions)
}

func (m *Sorted) Transitions(p StateId) chan WordStateWeight {
	ch := make(chan WordStateWeight)
	go func() {
		next := m.transitions[p]
		for _, i := range next[:len(next)-1] {
			ch <- i
		}
		close(ch)
	}()
	return ch
}

type byWord []WordStateWeight

func (s byWord) Len() int           { return len(s) }
func (s byWord) Less(i, j int) bool { return s[i].Word < s[j].Word }
func (s byWord) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// FIXME: a lot of redundant code in binary IO.

func (m *Sorted) header() (header []byte, err error) {
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
	numTransitions := make([]int, m.NumStates())
	for i, next := range m.transitions {
		numTransitions[i] = len(next) - 1
	}
	if err = enc.Encode(numTransitions); err != nil {
		return
	}
	header = buf.Bytes()
	return
}

func (m *Sorted) parseHeader(header []byte) (numTransitions []int, err error) {
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
	if err = dec.Decode(&numTransitions); err != nil {
		return
	}
	return
}

func (m *Sorted) WriteBinary(path string) (err error) {
	w, err := os.Create(path)
	if err != nil {
		return
	}
	defer w.Close()
	bw := byteblock.NewByteBlockWriter(w)
	if err = bw.WriteString(MAGIC_SORTED, 0); err != nil {
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

func IsSortedBinary(raw []byte) bool {
	bs := byteblock.NewByteBlockSlicer(raw)
	magic, err := bs.Slice()
	return err == nil && string(magic) == MAGIC_SORTED
}

func (m *Sorted) UnsafeParseBinary(raw []byte) error {
	bs := byteblock.NewByteBlockSlicer(raw)

	magic, err := bs.Slice()
	if err != nil {
		return err
	}
	if string(magic) != MAGIC_SORTED {
		return errors.New("not a FSLM binary file")
	}

	header, err := bs.Slice()
	if err != nil {
		return err
	}

	numTransitions, err := m.parseHeader(header)
	if err != nil {
		return err
	}

	entryBytes, err := bs.Slice()
	if err != nil {
		return err
	}
	var entrySlice []WordStateWeight
	entryBytesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&entryBytes))
	entrySliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&entrySlice))
	entrySliceHeader.Data = entryBytesHeader.Data
	entrySliceHeader.Len = entryBytesHeader.Len / int(unsafe.Sizeof(WordStateWeight{}))
	entrySliceHeader.Cap = entrySliceHeader.Len
	m.transitions = make([][]WordStateWeight, len(numTransitions))
	low := 0
	for i, n := range numTransitions {
		m.transitions[i] = entrySlice[low : low+n+1]
		low += n + 1
	}
	return nil
}
