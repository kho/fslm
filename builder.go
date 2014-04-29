package fslm

import (
	"fmt"
	"github.com/golang/glog"
	"io"
)

// Builder builds a Model from n-grams (e.g. estimated by SRILM). Must
// be constrcuted using NewBuilder().
type Builder struct {
	scale  float64
	vocab  *Vocab
	states []state
	nexts  []map[WordId]tgtWeight // TODO: memory hungry!
}

// NewBuilder constrcuts a new Builder. scale is the initial
// multiplier used to decide the number of buckets in final Model's
// hash map; when <= 1, a default multiplier of 1.5 is used. Larger
// multiplier generally speeds at final Model look up speed at the
// cost of using more memory. vocab is the base vocabulary to use for
// the resulting Model; it can be nil, in which case a default
// vocabulary with [<unk>, <s>, </s>] as [Unk, Bos, Eos] is
// created. Subsequent calls from Builder will not modify outside
// vocab (i.e. a copy is made when vocab != nil).
func NewBuilder(scale float64, vocab *Vocab) *Builder {
	var builder Builder
	if scale <= 1 {
		builder.scale = 1.5
	} else {
		builder.scale = scale
	}
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

// AddNgram adds an n-gram entry. The order of adding n-gram entry
// does not matter with regard to the final model size. For certain
// problematic input, warnings will be logged. The weights are changed
// to WEIGHT_LOG0 when they are no greater than the value of flag
// fslm.log0.
func (b *Builder) AddNgram(context []string, word string, weight Weight, backOff Weight) {
	if weight <= textLog0 {
		weight = WEIGHT_LOG0
	}
	if backOff <= textLog0 {
		backOff = WEIGHT_LOG0
	}
	if len(context) > 0 && word == b.vocab.BOS && weight > -10 {
		glog.Warningf("there is a non-unigram ending in %q with weight %g (such n-gram should have -inf weight or not occur in the LM)", word, weight)
	}
	if word == b.vocab.EOS && backOff != 0 {
		glog.Warningf("non-zero back-off %g for a n-gram ending in %q", backOff, word)
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
	// A large number of states may not have any out-going transition at
	// all. Delay construction of the map to save space.
	b.nexts = append(b.nexts, nil)
	return s
}

func (b *Builder) setTransition(p StateId, x WordId, q StateId, w Weight) {
	if b.nexts[p] == nil {
		b.nexts[p] = map[WordId]tgtWeight{}
	}
	b.nexts[p][x] = tgtWeight{q, w}
}

func (b *Builder) setBackOffWeight(p StateId, bow Weight) {
	b.states[p].BackOffWeight = bow
}

func (b *Builder) findNextState(p StateId, x WordId) StateId {
	if b.nexts[p] == nil {
		b.nexts[p] = map[WordId]tgtWeight{}
	}
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
// b. Subsequent calls to b.AddNgram() will have undefined behavior
// (probably panic and will definitely not give you correct Model).
func (b *Builder) Dump() *Model {
	b.link()
	return b.pruneMove()
}

// link links each state p to the first state q with at least one
// lexical transition along p's back-off chain.
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
			pBack = b.states[pBack].BackOffState
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

// pruneMove prunes the state space for immediately backing-off states
// and moves the contents to a real Model.
func (b *Builder) pruneMove() *Model {
	if glog.V(1) {
		glog.Infof("before pruning: %d states", len(b.states))
	}
	var m Model
	m.Vocab, b.vocab = b.vocab, nil
	// Compute mapping from old StateId to pruned StateId. Also counts
	// the total number of lexical transitions.
	oldToNew := make([]StateId, len(b.states))
	numTransitions := len(b.nexts[_STATE_EMPTY]) + len(b.nexts[_STATE_START])
	// _STATE_START and _STATE_EMPTY must be unchanged.
	oldToNew[_STATE_START] = _STATE_START
	nextId := StateId(_STATE_START + 1)
	for i, n := range b.nexts[_STATE_START+1:] {
		o := _STATE_START + 1 + StateId(i)
		numTransitions += len(n)
		if len(n) > 0 {
			oldToNew[o] = nextId
			nextId++
		} else {
			oldToNew[o] = STATE_NIL
		}
	}
	// Copy transitions and apply the mapping.
	m.transitions = NewMap(int(float64(numTransitions)*b.scale), 0)
	for i, n := range b.nexts {
		b.nexts[i] = nil // Allow GC to reclaim this map after this iteration.
		for x, qw := range n {
			q, w := qw.Tgt, qw.Weight
			if q != STATE_NIL {
				q = oldToNew[q]
				if q == STATE_NIL {
					s := &b.states[qw.Tgt]
					q = oldToNew[s.BackOffState]
					w += s.BackOffWeight
				}
			}
			*m.transitions.FindOrInsert(Key(newSrcWord(oldToNew[i], x))) = Value(tgtWeight{q, w})
		}
	}
	// Prune states.
	m.states, b.states = b.states, nil
	for o, s := range m.states[_STATE_START:] {
		n := oldToNew[StateId(o)+_STATE_START]
		if n != STATE_NIL {
			s.BackOffState = oldToNew[s.BackOffState]
			m.states[n] = s
		}
	}
	m.states = m.states[:nextId]
	if glog.V(1) {
		glog.Infof("after pruning: %d states", nextId)
		glog.Infof("there are %d lexical transitions", numTransitions)
	}
	return &m
}

// Graphviz visuallizes the current internal topology of the Builder.
func (b *Builder) Graphviz(w io.Writer) {
	fmt.Fprintln(w, "digraph {")
	fmt.Fprintln(w, "  // lexical transitions")
	for p, xqw := range b.nexts {
		for x, qw := range xqw {
			fmt.Fprintf(w, "  %d -> %d [label=%q]\n", p, qw.Tgt, fmt.Sprintf("%s : %g", b.vocab.StringOf(x), qw.Weight))
		}
	}
	fmt.Fprintln(w, "  // back-off transitions")
	for i, s := range b.states {
		fmt.Fprintf(w, "  %d -> %d [label=%q,style=dashed]\n", i, s.BackOffState, fmt.Sprintf("%g", s.BackOffWeight))
	}
	fmt.Fprintln(w, "}")
}
