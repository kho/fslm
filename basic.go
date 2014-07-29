package fslm

// Basic types and related constants.

import (
	"flag"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/kho/word"
)

// StateId represents a language model state.
type StateId uint32

const (
	STATE_NIL    StateId = ^StateId(0) // An invalid state.
	_STATE_EMPTY StateId = 0           // Models always uses state 0 for empty context.
	_STATE_START StateId = 1           // Models always uses state 1 for start.
)

// Weight is the floating point number type for log-probabilities.
type Weight float32

const WEIGHT_SIZE = 32 // The bit size of Weight.

func (w *Weight) String() string {
	return strconv.FormatFloat(float64(*w), 'g', -1, 32)
}

func (w *Weight) Set(s string) error {
	f, err := strconv.ParseFloat(s, 32)
	if err == nil {
		*w = Weight(f)
	}
	return err
}

// I seriously do not care about any platform that supports Go but
// does not support IEEE 754 infinity.
var (
	WEIGHT_LOG0 = Weight(math.Inf(-1))
	textLog0    = Weight(-99)
)

func init() {
	flag.Var(&textLog0, "fslm.log0", "treat weight <= this as log(0)")
}

type StateWeight struct {
	State  StateId
	Weight Weight
}

type WordStateWeight struct {
	Word   word.Id
	State  StateId
	Weight Weight
}

// Model is the general interface of an N-gram langauge model. It is
// mostly for convenience and the actual implementations should be
// prefered to speed up look-ups.
type Model interface {
	// Start returns the start state, i.e. the state with context
	// <s>. The user should never explicitly query <s>, which has
	// undefined behavior (see NextI).
	Start() StateId
	// NextI finds out the next state to go from p consuming x. x can
	// not be <s> or </s>, in which case the result is undefined, but
	// can be word.NIL. Any x that is not part of the model's vocabulary
	// is treated as OOV. The returned weight w is WEIGHT_LOG0 if and
	// only if unigram x is an OOV (note: although rare, it is possible
	// to have "<s> x" but not "x" in the LM, in which case "x" is also
	// considered an OOV when not occuring as the first token of a
	// sentence).
	NextI(p StateId, x word.Id) (q StateId, w Weight)
	// NextS is similar to NextI. s can be anything but <s> or </s>, in
	// which case the result is undefined.
	NextS(p StateId, x string) (q StateId, w Weight)
	// Final returns the final weight of "consuming" </s> from p. A
	// sentence query should finish with this to properly score the
	// *whole* sentence.
	Final(p StateId) Weight
	// Vocab returns the model's vocabulary and special sentence
	// boundary symbols.
	Vocab() (vocab *word.Vocab, bos, eos string, bosId, eosId word.Id)
}

// IterableModel is a language model whose states and transitions can
// be iterated.
type IterableModel interface {
	Model
	// NumStates returns the number of states. StateIds are always from
	// 0 to (the number of states - 1).
	NumStates() int
	// Transitions returns a channel that can be used to iterate over
	// the non-back-off transitions from a given state.
	Transitions(p StateId) chan WordStateWeight
	// BackOff returns the back off state and weight of p. The back off
	// state of the empty context is STATE_NIL and its weight is
	// arbitrary.
	BackOff(p StateId) (q StateId, w Weight)
}

// Graphviz prints out the finite-state topology of the model that can
// be visualized with Graphviz. Mostly for debugging; could be quite
// slow.
func Graphviz(m IterableModel, w io.Writer) {
	vocab, _, _, _, _ := m.Vocab()
	fmt.Fprintln(w, "digraph {")
	fmt.Fprintln(w, "  // lexical transitions")
	for i := 0; i < m.NumStates(); i++ {
		p := StateId(i)
		for xqw := range m.Transitions(p) {
			x, q, ww := xqw.Word, xqw.State, xqw.Weight
			fmt.Fprintf(w, "  %d -> %d [label=%q]\n", p, q, fmt.Sprintf("%s : %g", vocab.StringOf(x), ww))
		}
	}
	fmt.Fprintln(w, "  // back-off transitions")
	for i := 0; i < m.NumStates(); i++ {
		q, ww := m.BackOff(StateId(i))
		fmt.Fprintf(w, "  %d -> %d [label=%q,style=dashed]\n", i, q, fmt.Sprintf("%g", ww))
	}
	fmt.Fprintln(w, "}")
}

// A list of implemented models.
const (
	MODEL_HASHED = iota
	MODEL_SORTED
)

// Magic words for binary formats.
const (
	MAGIC_HASHED = "#fslm.hash"
	MAGIC_SORTED = "#fslm.sort"
)
