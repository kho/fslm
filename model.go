package fslm

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
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
