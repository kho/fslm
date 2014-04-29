package fslm

import (
	"bytes"
	"encoding/gob"
)

// Vocab is the mapping between strings and WordIds. Must be
// constructed using NewVocab() so that WORD_UNK, WORD_BOS and
// WORD_EOS are populated properly.
type Vocab struct {
	Unk, BOS, EOS string // For obvious reason the user should not modify these.
	id2str        []string
	str2id        map[string]WordId
}

func NewVocab(unk, bos, eos string) *Vocab {
	if unk == bos || unk == eos || bos == eos {
		panic("NewVocab: unk, bos, and eos can not be the same")
	}
	// We know WORD_UNK = 0...
	id2str := []string{WORD_UNK: unk, WORD_BOS: bos, WORD_EOS: eos}
	str2id := map[string]WordId{unk: WORD_UNK, bos: WORD_BOS, eos: WORD_EOS}
	return &Vocab{unk, bos, eos, id2str, str2id}
}

// Copy returns a new Vocab that can be modified without changing v.
func (v *Vocab) Copy() *Vocab {
	var c = *v

	// We must copy this because if the user makes multiple copies and
	// modifies each of them, the shared slice will be in a corrupted
	// state.
	c.id2str = make([]string, len(v.id2str))
	copy(c.id2str, v.id2str)

	c.str2id = map[string]WordId{}
	for k, v := range v.str2id {
		c.str2id[k] = v
	}

	return &c
}

// Bound returns the largest WordId + 1.
func (v *Vocab) Bound() WordId { return WordId(len(v.id2str)) }

// IdOf looks up the WordId of the given string. If s is not present,
// WORD_UNK is returned.
func (v *Vocab) IdOf(s string) WordId { return v.str2id[s] }

// StringOf looks up the string of the given WordId. Only safe when i
// is WORD_UNK, WORD_BOS, WORD_EOS or returned from either IdOf() or
// IdOrAdd().
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
	if err = enc.Encode(v.Unk); err != nil {
		return
	}
	if err = enc.Encode(v.BOS); err != nil {
		return
	}
	if err = enc.Encode(v.EOS); err != nil {
		return
	}
	if err = enc.Encode(v.id2str); err != nil {
		return
	}
	if err = enc.Encode(v.str2id); err != nil {
		return
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary deserializes a Vocab. The Vocab will be in an
// invalid state an error is returned.
func (v *Vocab) UnmarshalBinary(data []byte) (err error) {
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err = dec.Decode(&v.Unk); err != nil {
		return
	}
	if err = dec.Decode(&v.BOS); err != nil {
		return
	}
	if err = dec.Decode(&v.EOS); err != nil {
		return
	}
	if err = dec.Decode(&v.id2str); err != nil {
		return
	}
	if err = dec.Decode(&v.str2id); err != nil {
		return
	}
	return nil
}
