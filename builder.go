package fslm

import (
	"fmt"
	"github.com/golang/glog"
	"io"
)

// Builder builds a Model from n-grams (e.g. estimated by SRILM). Must
// be constrcuted using NewBuilder().
type Builder struct {
	scale        float64
	vocab        *Vocab
	bos, eos     string
	bosId, eosId WordId
	transitions  []*xqwMap
	backoff      []StateWeight
}

// NewBuilder constrcuts a new Builder. scale is the initial
// multiplier used to decide the number of buckets in final Model's
// hash map; when <= 1, a default multiplier of 1.5 is used. Larger
// multiplier generally speeds at final Model look up speed at the
// cost of using more memory. vocab is the base vocabulary to use for
// the resulting Model; it can be nil, in which case a default
// vocabulary with [<s>, </s>] as the first two words is
// created. Otherwise, bos and eos are used to query the sentence
// boundary symbols from the vocab. Subsequent calls from Builder will
// not modify outside vocab (i.e. a copy is made when vocab != nil).
func NewBuilder(scale float64, vocab *Vocab, bos, eos string) *Builder {
	var builder Builder

	if scale <= 1 {
		builder.scale = 1.5
	} else {
		builder.scale = scale
	}

	if vocab == nil {
		vocab = NewVocab([]string{"<s>", "</s>"})
		bos = "<s>"
		eos = "</s>"
	} else {
		vocab = vocab.Copy()
	}
	builder.vocab = vocab
	if bos != eos {
		builder.bos = bos
		builder.eos = eos
	} else {
		glog.Fatalf("begin-of-sentence and end-of-sentence are the same word %q", bos)
	}

	if builder.bosId = vocab.IdOf(bos); builder.bosId == WORD_NIL {
		glog.Fatalf("%q not in vocabulary", bos)
	}
	if builder.eosId = vocab.IdOf(eos); builder.eosId == WORD_NIL {
		glog.Fatalf("%q not in vocabulary", eos)
	}

	// _STATE_EMPTY and _STATE_START.
	builder.newState()
	builder.newState()
	builder.setTransition(_STATE_EMPTY, builder.bosId, _STATE_START, 0)
	// We need to ensure _STATE_START has some bucket because it is the
	// only state that is not pre-walked.
	builder.transitions[_STATE_START] = newXqwMap(0, 0)
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

	if len(context) > 0 {
		if context[0] == b.eos {
			glog.Fatalf("end-of-sentence in context %q", context)
		}
		for _, i := range context[1:] {
			if i == b.bos {
				glog.Fatalf("begin-of-sentence not in the beginning of context %q", context)
			}
			if i == b.eos {
				glog.Fatalf("end-of-sentence in context %q", context)
			}
		}
	}

	if len(context) > 0 && word == b.bos && weight > -10 {
		glog.Warningf("there is a non-unigram ending in %q with weight %g (such n-gram should have -inf weight or not occur in the LM)", word, weight)
	}
	if word == b.eos && backOff != 0 {
		glog.Warningf("non-zero back-off %g for a n-gram ending in %q", backOff, word)
	}

	p := b.findState(_STATE_EMPTY, context)
	x := b.vocab.IdOrAdd(word)
	q := STATE_NIL
	// Only use a valid destination state when word is not </s>.
	if x != b.eosId {
		q = b.findNextState(p, x)
		b.setBackOffWeight(q, backOff)
	}
	b.setTransition(p, x, q, weight)
}

func (b *Builder) newState() StateId {
	s := StateId(len(b.backoff))
	// A large number of states may not have any out-going transition at
	// all. Delay construction of the map to save space.
	b.transitions = append(b.transitions, nil)
	// Back-off is initialized to STATE_NIL to signify an "unknown"
	// back-off.
	b.backoff = append(b.backoff, StateWeight{STATE_NIL, 0})
	return s
}

func (b *Builder) setTransition(p StateId, x WordId, q StateId, w Weight) {
	if b.transitions[p] == nil {
		b.transitions[p] = newXqwMap(0, 0)
	}
	*b.transitions[p].FindOrInsert(x) = StateWeight{q, w}
}

func (b *Builder) setBackOffWeight(p StateId, bow Weight) {
	b.backoff[p].Weight = bow
}

func (b *Builder) findNextState(p StateId, x WordId) StateId {
	if b.transitions[p] == nil {
		b.transitions[p] = newXqwMap(0, 0)
	}
	qw := b.transitions[p].Find(x)
	if qw != nil {
		return qw.State
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
	// For safety.
	if _STATE_EMPTY != 0 {
		glog.Fatalf("this assumes _STATE_EMPTY == 0; got %d", _STATE_EMPTY)
	}
	if _STATE_START != 1 {
		glog.Fatalf("this assumes _STATE_START == 1; got %d", _STATE_START)
	}
	b.link()
	return b.pruneMove()
}

// link links each state p to the first state q with at least one
// lexical transition along p's back-off chain.
func (b *Builder) link() {
	// Children of _STATE_EMPTY directly backs off the _STATE_EMPTY.
	for xqw := range b.transitions[_STATE_EMPTY].Range() {
		q := xqw.Value.State
		if q != STATE_NIL {
			b.backoff[q].State = _STATE_EMPTY
		}
	}
	// States are created with STATE_NIL as the default back-off. Except
	// for _STATE_EMPTY, having a STATE_NIL back-off means the back-off
	// is yet to be computed.
	for i, es := range b.transitions[_STATE_EMPTY+1:] {
		if es != nil {
			for xqw := range es.Range() {
				p, x, q := StateId(i+1), WordId(xqw.Key), xqw.Value.State
				if q != STATE_NIL {
					b.linkTransition(p, x, q)
				}
			}
		}
	}
}

// linkTransition recursively link q to the lowest back-off state with
// at least one lexical transition. q must not be _STATE_EMPTY. This
// function might change q's back-off weight when the final back-off
// state is not q's immediately back-off.
func (b *Builder) linkTransition(p StateId, x WordId, q StateId) (StateId, Weight) {
	qBackOff := &b.backoff[q]
	if qBackOff.State == STATE_NIL {
		// Find the next back-off state.
		pBack := b.backoff[p].State
		qwBack := b.transitions[pBack].Find(x)
		for qwBack == nil && pBack != _STATE_EMPTY {
			pBack = b.backoff[pBack].State
			qwBack = b.transitions[pBack].Find(x)
		}
		if qwBack != nil {
			qBack := qwBack.State
			// pBack is not STATE_NIL; qBack is not _STATE_EMPTY. We can go
			// back further.
			qBackBack, w := b.linkTransition(pBack, x, qBack)
			if b.transitions[qBack] == nil { // = .Size() == 0 (for states other than _STATE_START, we only create the map at first insertion).
				qBackOff.State = qBackBack
				// We are skipping the transition from qBack to qBackBack,
				// thus its weight needs to be included in our back-off weight
				// as well.
				qBackOff.Weight += w
			} else {
				qBackOff.State = qBack
			}
		} else {
			qBackOff.State = _STATE_EMPTY
		}
	}
	return qBackOff.State, qBackOff.Weight
}

// pruneMove prunes the state space for immediately backing-off states
// and moves the contents to a real Model.
func (b *Builder) pruneMove() *Model {
	if glog.V(1) {
		glog.Infof("before pruning: %d states", len(b.backoff))
	}
	var m Model
	m.Vocab, b.vocab = b.vocab, nil // Steal!
	m.BOS, m.EOS, m.BOSId, m.EOSId = b.bos, b.eos, b.bosId, b.eosId
	// Compute mapping from old StateId to pruned StateId.
	oldToNew := make([]StateId, len(b.backoff))
	// _STATE_EMPTY and _STATE_START must be unchanged.
	oldToNew[_STATE_EMPTY] = _STATE_EMPTY
	oldToNew[_STATE_START] = _STATE_START
	nextId := StateId(_STATE_START + 1)
	for i, es := range b.transitions[_STATE_START+1:] {
		o := _STATE_START + 1 + StateId(i)
		if es != nil { // = .Size() != 0 (for states other than _STATE_START, we only create the map at the first insertion).
			oldToNew[o] = nextId
			nextId++
		} else {
			oldToNew[o] = STATE_NIL
		}
	}
	m.transitions = make([]xqwBuckets, nextId)
	// Copy transitions and apply the mapping.
	for i, es := range b.transitions {
		if es == nil { // = .Size() == 0 (for states other than _STATE_START)
			continue
		}
		// Steal Builder's data.
		b.transitions[i] = nil
		// Walk over the buckets. If it holds an edge, pre-walk to the
		// proper destination state. If it does not hold an edge, set it
		// to the back-off.
		backoff := b.backoff[i]
		if backoff.State != STATE_NIL {
			backoff.State = oldToNew[backoff.State]
		}
		buckets := es.buckets
		for j, xqw := range buckets {
			if xqw.Key != WORD_NIL {
				q, w := xqw.Value.State, xqw.Value.Weight
				if q != STATE_NIL {
					oldQ := q
					q = oldToNew[oldQ]
					if q == STATE_NIL {
						s := &b.backoff[oldQ]
						q = oldToNew[s.State]
						w += s.Weight
					}
				}
				xqw.Value = StateWeight{q, w}
			} else {
				xqw.Value = backoff
			}
			buckets[j] = xqw
		}
		m.transitions[oldToNew[i]] = buckets
	}
	// Free last two pieces of Builder data.
	b.backoff = nil
	b.transitions = nil
	if glog.V(1) {
		glog.Infof("after pruning: %d states", nextId)
	}
	return &m
}

// Graphviz visuallizes the current internal topology of the Builder.
func (b *Builder) Graphviz(w io.Writer) {
	fmt.Fprintln(w, "digraph {")
	fmt.Fprintln(w, "  // lexical transitions")
	for p, es := range b.transitions {
		if es != nil {
			for xqw := range es.Range() {
				x, qw := WordId(xqw.Key), xqw.Value
				fmt.Fprintf(w, "  %d -> %d [label=%q]\n", p, qw.State, fmt.Sprintf("%s : %g", b.vocab.StringOf(x), qw.Weight))
			}
		}
	}
	fmt.Fprintln(w, "  // back-off transitions")
	for i, s := range b.backoff {
		fmt.Fprintf(w, "  %d -> %d [label=%q,style=dashed]\n", i, s.State, fmt.Sprintf("%g", s.Weight))
	}
	fmt.Fprintln(w, "}")
}
