package fslm

import (
	"fmt"
	"io"
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

// Builder builds a Model from n-grams (e.g. estimated by SRILM). Must
// be constrcuted using NewBuilder().
type Builder struct {
	vocab  *Vocab
	states []state
	nexts  []map[WordId]tgtWeight
}

// NewBuilder constrcuts a new Builder. vocab is the base vocabulary
// to use for the resulting Model; it can be nil, in which case a
// vocabulary with [<unk>, <s>, </s>] as [Unk, Bos, Eos]. Subsequent
// calls from Builder will not modify vocab (i.e. a copy is made when
// vocab != nil).
func NewBuilder(vocab *Vocab) *Builder {
	var builder Builder
	if vocab == nil {
		builder.vocab = NewVocab("<unk>", "<s>", "</s>")
	} else {
		builder.vocab = vocab.Copy()
	}
	// _STATE_EMPTY and _STATE_START.
	builder.newState()
	builder.newState()
	builder.setTransition(_STATE_EMPTY, WORD_BOS, _STATE_START, 0)
	return &builder
}

// AddNGram adds an n-gram entry. context must be within proper order
// or this will panic. For other problematic input, warnings will be
// logged.
func (b *Builder) AddNGram(context []string, word string, weight Weight, backOff Weight) {
	if len(context) > 0 && word == b.vocab.BOS && weight > -10 {
		log.Printf("there is a non-unigram ending in %q with weight %g (such n-gram should have -inf weight or not occur in the LM)", word, weight)
	}
	if word == b.vocab.EOS && backOff != 0 {
		log.Printf("non-zero back-off %g for a n-gram ending in %q", backOff, word)
	}

	p := b.findState(_STATE_EMPTY, context)
	x := b.vocab.IdOrAdd(word)
	q := STATE_NIL
	// Only use a valid destination state when word is not EOS.
	if x != WORD_EOS {
		q = b.findNextState(p, x)
		b.setBackOffWeight(q, backOff)
	}
	b.setTransition(p, x, q, weight)
}

func (b *Builder) newState() StateId {
	s := StateId(len(b.states))
	// Back-off is initialized to STATE_NIL to signify an "unknown"
	// back-off.
	b.states = append(b.states, state{STATE_NIL, 0})
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
	b.link()
	return b.move()
}

// link links each state p to the first state q with at least one
// lexical transition along p' back-off chain.
func (b *Builder) link() {
	// For safety.
	if _STATE_EMPTY != 0 || _STATE_START != 1 {
		panic("this assumes _STATE_EMPTY == 0 and _STATE_START == 1")
	}
	// Children of _STATE_EMPTY directly backs off the _STATE_EMPTY.
	for _, qw := range b.nexts[_STATE_EMPTY] {
		if qw.Tgt != STATE_NIL {
			b.states[qw.Tgt].BackOffState = _STATE_EMPTY
		}
	}
	// States are created with STATE_NIL as the default back-off. Except
	// for _STATE_EMPTY, having a STATE_NIL back-off means the back-off
	// is yet to be computed.
	for i, next := range b.nexts[1:] {
		for x, qw := range next {
			p, q := StateId(i+1), qw.Tgt
			if q != STATE_NIL {
				b.linkTransition(p, x, q)
			}
		}
	}
}

// linkTransition recursively link q to the lowest back-off state with
// at least one lexical transition. q must not be _STATE_EMPTY. This
// function might change q's back-off weight when the final back-off
// state is not q's immediately back-off.
func (b *Builder) linkTransition(p StateId, x WordId, q StateId) (StateId, Weight) {
	qState := &b.states[q]
	if qState.BackOffState == STATE_NIL {
		// Find the next back-off state.
		pBack := b.states[p].BackOffState
		qwBack, ok := b.nexts[pBack][x]
		for !ok && pBack != _STATE_EMPTY {
			pBack := b.states[pBack].BackOffState
			qwBack, ok = b.nexts[pBack][x]
		}
		if ok {
			qBack := qwBack.Tgt
			// pBack is not STATE_NIL; qBack is not _STATE_EMPTY. We can go
			// back further.
			qBackBack, w := b.linkTransition(pBack, x, qBack)
			if len(b.nexts[qBack]) == 0 {
				qState.BackOffState = qBackBack
				// We are skipping the transition from qBack to qBackBack,
				// thus its weight needs to be included in our back-off weight
				// as well.
				qState.BackOffWeight += w
			} else {
				qState.BackOffState = qBack
			}
		} else {
			qState.BackOffState = _STATE_EMPTY
		}
	}
	return qState.BackOffState, qState.BackOffWeight
}

func (b *Builder) move() *Model {
	var m Model
	m.Vocab, b.vocab = b.vocab, nil
	m.states, b.states = b.states, nil
	m.transitions = map[srcWord]tgtWeight{}
	for i, n := range b.nexts {
		b.nexts[i] = nil // Allow GC to reclaim this map after this iteration.
		for x, qw := range n {
			m.transitions[newSrcWord(StateId(i), x)] = tgtWeight{qw.Tgt, qw.Weight}
		}
	}
	return &m
}
