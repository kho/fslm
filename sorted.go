package fslm

import (
	"github.com/kho/word"
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
