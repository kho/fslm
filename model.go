package fslm

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
)

// Model is a finite-state representation of a n-gram language model.
type Model struct {
	// The vocabulary of the model. Don't modify this. If you need to
	// have a vocab based on this, make a copy using Vocab.Copy().
	Vocab *Vocab
	// The first two states are always _STATE_START and
	// _STATE_EMPTY. Any properly constructed Model must have these two
	// states.
	states []state
	// There are two kinds of transitions:
	//
	// (1) A lexical transition that consumes an actual word (i.e. not
	// WORD_UNK, WORD_EOS). This leads to a valid state with some
	// weight. Note we allow transition from empty consuming
	// WORD_BOS. This transition should have WEIGHT_LOG0 anyway
	// (e.g. thoes built from SRILM) so keeping it doesn't cause much
	// trouble.
	//
	// (2) A final transition that consumes WORD_EOS. This gives the
	// final weight but always leads to an undefined state.
	transitions transitionMap
}

// Size returns the size of the model in several aspects.
func (m *Model) Size() (numStates, numTransitions, vocabSize int) {
	return len(m.states), len(m.transitions), int(m.Vocab.Bound())
}

// Start returns the start state, i.e. the state with context
// [WORD_BOS]. The user should never explicitly querying WORD_BOS,
// which has undefined behavior (see NextI).
func (m *Model) Start() StateId {
	return _STATE_START
}

// NextI finds out the next state to go from p consuming i. i can not
// be WORD_BOS or WORD_EOS, in which case the result is undefined, but
// can be WORD_UNK. Any i that is not part of m.Vocab is treated as
// OOV. The returned weight w is WEIGHT_LOG0 if and only if unigram i
// is an OOV (note: although rare, it is possible to have "<s> x" but
// not "x" in the LM, in which case "x" is also considered an OOV when
// not occuring as the first token of a sentence).
func (m *Model) NextI(p StateId, i WordId) (q StateId, w Weight) {
	// TODO: This might not be really useful given how rare we hit <unk>
	// and the code below behaves as expected in case of <unk>.
	//
	// For <unk> we know immediately where to go.
	// if i == WORD_UNK {
	// 	return _STATE_EMPTY, WEIGHT_LOG0
	// }

	// Try backing off until we find the n-gram or hit empty state.
	next, ok := m.transitions[newSrcWord(p, i)]
	for !ok && p != _STATE_EMPTY {
		s := m.states[p]
		p = s.BackOffState
		w += s.BackOffWeight
		next, ok = m.transitions[newSrcWord(p, i)]
	}
	if ok {
		q = next.Tgt
		w += next.Weight
	} else {
		q = _STATE_EMPTY
		w = WEIGHT_LOG0
	}
	return
}

// NextS is similar to NextI. s can be anything but "<s>" or "</s>",
// in which case the result is undefined.
func (m *Model) NextS(p StateId, s string) (q StateId, w Weight) {
	return m.NextI(p, m.Vocab.IdOf(s))
}

// Final returns the final weight of "consuming" WORD_EOS from p. A
// sentence query should finish with this to properly score the
// *whole* sentence.
func (m *Model) Final(p StateId) Weight {
	_, w := m.NextI(p, WORD_EOS)
	return w
}

// Graphviz prints out the finite-state topology of the model that can
// be visualized with Graphviz. Mostly for debugging; could be quite
// slow.
func (m *Model) Graphviz(w io.Writer) {
	fmt.Fprintln(w, "digraph {")
	fmt.Fprintln(w, "  // lexical transitions")
	for px, qw := range m.transitions {
		fmt.Fprintf(w, "  %d -> %d [label=%q]\n", px.Src(), qw.Tgt, fmt.Sprintf("%s : %g", m.Vocab.StringOf(px.Word()), qw.Weight))
	}
	fmt.Fprintln(w, "  // back-off transitions")
	for i, s := range m.states {
		fmt.Fprintf(w, "  %d -> %d [label=%q,style=dashed]\n", i, s.BackOffState, fmt.Sprintf("%g", s.BackOffWeight))
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
	if err = enc.Encode(m.states); err != nil {
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
	if err = dec.Decode(&m.states); err != nil {
		return
	}
	if err = dec.Decode(&m.transitions); err != nil {
		return
	}
	return nil
}

type state struct {
	BackOffState  StateId
	BackOffWeight Weight
}

type srcWord uint64 // map queries are much faster with this than a struct of two uint32s.

func (i srcWord) Src() StateId {
	return StateId(i >> 32)
}

func (i srcWord) Word() WordId {
	return WordId(i & 0xFFFFFFFF)
}

func newSrcWord(s StateId, w WordId) srcWord {
	return srcWord(s)<<32 | srcWord(w)
}

type tgtWeight struct {
	Tgt    StateId
	Weight Weight
}

// TODO: implement probing instead of using stdlib map.
type transitionMap map[srcWord]tgtWeight
