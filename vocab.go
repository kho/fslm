package fslm

import (
	"bytes"
	"encoding/gob"
)

// Vocab is the mapping between strings and WordIds. Must be
// constructed using NewVocab().
type Vocab struct {
	id2str []string
	str2id map[string]WordId
}

func NewVocab(initWords []string) *Vocab {
	id2str := make([]string, len(initWords))
	copy(id2str, initWords)
	str2id := map[string]WordId{}
	for i, s := range id2str {
		str2id[s] = WordId(i)
	}
	if len(id2str) != len(str2id) {
		panic("there are duplicate words in the initial word list")
	}
	return &Vocab{id2str, str2id}
}

// Copy returns a new Vocab that can be modified without changing v.
func (v *Vocab) Copy() *Vocab {
	return NewVocab(v.id2str)
}

// Bound returns the largest WordId + 1.
func (v *Vocab) Bound() WordId { return WordId(len(v.id2str)) }

// IdOf looks up the WordId of the given string. If s is not present,
// WORD_NIL is returned.
func (v *Vocab) IdOf(s string) WordId {
	id, ok := v.str2id[s]
	if ok {
		return id
	}
	return WORD_NIL
}

// StringOf looks up the string of the given WordId. i must be a valid
// WordId already added to v.
func (v *Vocab) StringOf(i WordId) string { return v.id2str[i] }

// IdOrAdd looks up s to find its corresponding WordId. When s is not
// present, it adds it to the vocabulary. This is not thread-safe
// since it may modify the vocabulary. The returned WordId is WORD_UNK
// if and only if s is v.Unk().
func (v *Vocab) IdOrAdd(s string) WordId {
	i, ok := v.str2id[s]
	if !ok {
		i = v.Bound()
		v.id2str = append(v.id2str, s)
		v.str2id[s] = i
	}
	return i
}

// MarshalBinary serializes a Vocab. Usually Vocab are a few MBs at
// most so this should be fine.
func (v *Vocab) MarshalBinary() (data []byte, err error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err = enc.Encode(v.id2str); err != nil {
		return
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary deserializes a Vocab. The Vocab will be in an
// invalid state an error is returned.
func (v *Vocab) UnmarshalBinary(data []byte) (err error) {
	var id2str []string
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err = dec.Decode(&id2str); err != nil {
		return
	}
	*v = *NewVocab(id2str)
	return nil
}
