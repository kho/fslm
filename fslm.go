package fslm

import (
	"fmt"
	"log"
)

type WordId uint32

const (
	WORD_UNK WordId = 0
	WORD_BOS WordId = 1
	WORD_EOS WordId = 2
)

// Vocab is the mapping between strings and WordIds. Must be
// constructed using NewVocab() so that WORD_UNK, WORD_BOS and
// WORD_EOS are populated properly.
type Vocab struct {
	Unk, Bos, Eos string // For obvious reason the user should not modify these.
	id2str        []string
	str2id        map[string]WordId
}

func NewVocab(unk, bos, eos string) *Vocab {
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

type StateId uint32

const (
	STATE_NIL StateId = ^StateId(0) // An invalid state.
)

type Weight float32

const (
	WEIGHT_LOG0 Weight = -99 // Replacement of -inf following the convention of SRILM.
)

type state struct {
	BackOffState  StateId
	BackOffWeight Weight
}

// map queries are much faster with this than a struct of two uint32s.
type srcWord uint64

func newSrcWord(s StateId, w WordId) srcWord {
	return srcWord(s)<<32 | srcWord(w)
}

type tgtWeight struct {
	Tgt    StateId
	Weight Weight
}

// TODO: implement probing instead of using stdlib map.
type transitionMap map[srcWord]tgtWeight

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

const (
	_STATE_EMPTY StateId = 0
	_STATE_START StateId = 1
)

// Start returns the start state, i.e. the state with context
// [WORD_BOS]. The user should never explicitly querying WORD_BOS,
// which will be treated as an OOV and breaks the context.
func (m *Model) Start() StateId {
	return _STATE_START
}

// NextI finds out the next state to go from p consuming i. i can not
// be WORD_BOS or WORD_EOS, in which case the result is undefined, but
// can be WORD_UNK. The returned weight w is WEIGHT_LOG0 if and only
// if unigram i is an OOV (note: although rare, it is possible to have
// "<s> x" but not "x" in the LM, in which case "x" is also considered
// an OOV).
func (m *Model) NextI(p StateId, i WordId) (q StateId, w Weight) {
	// For <unk> we know immediately where to go.
	if i == WORD_UNK {
		return _STATE_EMPTY, WEIGHT_LOG0
	}
	// Otherwise try backing off until we find the n-gram or hit empty
	// state.
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

// Final returns the final weight of "consuming" WORD_EOS from p.
func (m *Model) Final(p StateId) Weight {
	_, w := m.NextI(p, WORD_EOS)
	return w
}

// Builder builds a Model from n-grams (e.g. estimated by SRILM). Must
// be constrcuted using NewBuilder().
type Builder struct {
	order  int
	vocab  *Vocab
	states []state
	nexts  []map[WordId]tgtWeight
}

// NewBuilder constrcuts a new Builder. order is the maximum order of
// n-grams (i.e. n-1). vocab is the base vocabulary to use for the
// resulting Model; it can be nil, in which case a vocabulary with
// [<unk>, <s>, </s>] as [Unk, Bos, Eos]. Subsequent calls from
// Builder will not modify vocab (i.e. a copy is made when vocab !=
// nil).
func NewBuilder(order int, vocab *Vocab) *Builder {
	if order <= 0 {
		panic(fmt.Sprintf("LM order should be postive; not %d", order))
	}

	var builder Builder
	builder.order = order
	if vocab == nil {
		builder.vocab = NewVocab("<unk>", "<s>", "</s>")
	} else {
		builder.vocab = vocab.Copy()
	}
	// _STATE_EMPTY and _STATE_START.
	builder.newState()
	builder.newState()
	return &builder
}

// AddNGram adds an n-gram entry. context must be within proper order
// or this will panic. For other problematic input, warnings will be
// logged.
func (b *Builder) AddNGram(context []string, word string, weight Weight, backOff Weight) {
	if len(context) > b.order {
		panic(fmt.Sprintf("adding %d-gram to order-%d LM: %q -> %q", len(context)+1, b.order, context, word))
	}
	if len(context) > 0 && word == b.vocab.Bos && weight > -10 {
		log.Printf("there is a non-unigram ending in %q with weight %g (such n-gram should have -inf weight or not occur in the LM)", word, weight)
	}
	if word == b.vocab.Eos && backOff != 0 {
		log.Printf("non-zero back-off %g for a n-gram ending in %q", backOff, word)
	}

	p := b.findState(_STATE_EMPTY, context)
	wordI := b.vocab.IdOrAdd(word)
	if len(context) < b.order {
		q := b.findNextState(p, wordI)
		b.setTransition(p, wordI, q, weight)
		b.setBackOffWeight(q, backOff)
	} else {
		q := b.findNextState(b.findState(_STATE_EMPTY, context[:len(context)-1]), wordI)
		b.setTransition(p, wordI, q, weight)
		if backOff != 0 {
			log.Printf("ignoring the non-zero back-off weight of a highest-order n-gram: %q -> %q", context, word)
		}
	}
}

func (b *Builder) newState() StateId {
	s := StateId(len(b.states))
	// Initialize the back-off state to 0, which is the back-off state
	// for unigram contexts.
	//
	// TODO: 0 or WEIGHT_LOG0 for back-off weight?
	b.states = append(b.states, state{})
	b.nexts = append(b.nexts, map[WordId]tgtWeight{})
	return s
}

func (b *Builder) setTransition(p StateId, x WordId, q StateId, w Weight) {
	b.nexts[p][x] = tgtWeight{q, w}
}

func (b *Builder) setBackOffWeight(p StateId, bow Weight) {
	b.states[p].BackOffWeight = bow
}

func (b *Builder) findNextState(p StateId, x WordId) StateId {
	qw, ok := b.nexts[p][x]
	if ok {
		return qw.Tgt
	}
	q := b.newState()
	b.setTransition(p, x, q, 0)
	return q
}

func (b *Builder) findState(p StateId, ws []string) StateId {
	for _, w := range ws {
		p = b.findNextState(p, b.vocab.IdOrAdd(w))
	}
	return p
}

// Dump creates the result Model and invalidates the internal data of
// b. Subsequent calls to b.AddNGram() will have undefined behavior
// (probably panic).
func (b *Builder) Dump() *Model {
	b.linkBackOff()
	return b.move()
}

func (b *Builder) linkBackOff() {
	b.states[_STATE_EMPTY].BackOffState = STATE_NIL
	// For safety...
	if _STATE_EMPTY != 0 || _STATE_START != 1 {
		panic("this assumes _STATE_EMPTY == 0 and _STATE_START == 1")
	}
	// newState has already set up the back-off states for children of
	// _STATE_EMPTY. So start with the next guy (i.e. _STATE_START).
	for i, s := range b.states[_STATE_START:] {
		for x, qw := range b.nexts[StateId(i)+_STATE_START] {
			q := qw.Tgt
			pBack := s.BackOffState
			pBackQw, ok := b.nexts[pBack][x]
			for !ok && pBack != _STATE_EMPTY {
				pBack = b.states[pBack].BackOffState
				pBackQw, ok = b.nexts[pBack][x]
			}
			b.states[q].BackOffState = pBackQw.Tgt
		}
	}
}

func (b *Builder) move() *Model {
	var m Model
	m.Vocab, b.vocab = b.vocab, nil
	m.states, b.states = b.states, nil
	m.transitions = map[srcWord]tgtWeight{}
	for i, n := range b.nexts {
		b.nexts[i] = nil // Allow GC to reclaim this map after this iteration.
		for x, qw := range n {
			m.transitions[newSrcWord(StateId(i), x)] = qw
		}
	}
	return &m
}
