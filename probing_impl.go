package fslm

import (
	"bytes"
	"encoding/gob"
)

type xqwEntry struct {
	Key   WordId
	Value StateWeight
}

type xqwMap struct {
	buckets               xqwBuckets
	numEntries, threshold int
}

func newXqwMap(initNumBuckets int, maxUsed float64) *xqwMap {
	if initNumBuckets < 2 {
		initNumBuckets = 2
	}
	if maxUsed <= 0 || maxUsed >= 1 {
		maxUsed = 0.8
	}
	// threshold = min(max(1, (initNumBuckets-1) * maxUsed), initNumBuckets-1)
	threshold := int(float64(initNumBuckets-1) * maxUsed)
	if threshold < 1 {
		threshold = 1
	}
	if threshold > initNumBuckets-1 {
		threshold = initNumBuckets - 1
	}
	return &xqwMap{xqwInitBuckets(initNumBuckets), 0, threshold}
}

func (m *xqwMap) Size() int {
	return m.numEntries
}

func (m *xqwMap) Find(k WordId) *StateWeight {
	return m.buckets.Find(k)
}

func (m *xqwMap) FindOrInsert(k WordId) *StateWeight {
	e := m.buckets.FindEntry(k)
	if e.Key != WORD_NIL {
		return &e.Value
	}
	// Need to insert.
	if m.numEntries >= m.threshold {
		m.double()
		e = m.buckets.nextAvailable(k)
	}
	*e = xqwEntry{Key: k}
	m.numEntries++
	return &e.Value
}

func (m *xqwMap) Range() chan xqwEntry {
	return m.buckets.Range()
}

func (m *xqwMap) MarshalBinary() (data []byte, err error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err = enc.Encode(m.buckets); err != nil {
		return
	}
	if err = enc.Encode(m.numEntries); err != nil {
		return
	}
	if err = enc.Encode(m.threshold); err != nil {
		return
	}
	return buf.Bytes(), nil
}

func (m *xqwMap) UnmarshalBinary(data []byte) (err error) {
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err = dec.Decode(&m.buckets); err != nil {
		return
	}
	if err = dec.Decode(&m.numEntries); err != nil {
		return
	}
	if err = dec.Decode(&m.threshold); err != nil {
		return
	}
	return nil
}

func (m *xqwMap) double() {
	buckets := xqwInitBuckets(len(m.buckets) * 2)
	for _, e := range m.buckets {
		k := e.Key
		if !WordIdEqual(k, WORD_NIL) {
			dst := buckets.nextAvailable(k)
			*dst = e
		}
	}
	m.buckets = buckets
	m.threshold *= 2
}

type xqwBuckets []xqwEntry

func xqwInitBuckets(n int) xqwBuckets {
	s := make(xqwBuckets, n)
	for i := range s {
		s[i].Key = WORD_NIL
	}
	return s
}

func (b xqwBuckets) Size() (n int) {
	for _, e := range b {
		if e.Key != WORD_NIL {
			n++
		}
	}
	return
}

// var numLookUps, numCollisions int

func (b xqwBuckets) Find(k WordId) (v *StateWeight) {
	// numLookUps++
	i := b.start(k)
	for {
		// Maybe switch to range to trade 1 bound check for 1 copy?
		ei := &b[i]
		ki := ei.Key
		if WordIdEqual(ki, k) {
			return &ei.Value
		}
		if WordIdEqual(ki, WORD_NIL) {
			return nil
		}
		// numCollisions++
		i++
		if i == len(b) {
			i = 0
		}
	}
}

func (b xqwBuckets) FindEntry(k WordId) *xqwEntry {
	i := b.start(k)
	for {
		ei := &b[i]
		ki := ei.Key
		if WordIdEqual(ki, k) || WordIdEqual(ki, WORD_NIL) {
			return ei
		}
		i++
		if i == len(b) {
			i = 0
		}
	}
}

func (b xqwBuckets) Range() chan xqwEntry {
	ch := make(chan xqwEntry)
	go func() {
		for _, e := range b {
			if e.Key != WORD_NIL {
				ch <- e
			}
		}
		close(ch)
	}()
	return ch
}

func (b xqwBuckets) start(k WordId) int {
	return int(WordIdHash(k) % uint(len(b)))
}

func (b xqwBuckets) nextAvailable(k WordId) *xqwEntry {
	i := b.start(k)
	for {
		ei := &b[i]
		if WordIdEqual(ei.Key, WORD_NIL) {
			return ei
		}
		i++
		if i == len(b) {
			i = 0
		}
	}
}