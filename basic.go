package fslm

// Basic types and related constants.

import (
	"flag"
	"math"
	"strconv"
)

// WordId represents word identity.
type WordId uint32

const (
	WORD_UNK WordId = 0 // Vocab always uses this for <unk>.
	WORD_BOS WordId = 1 // Vocab always uses this for <s>.
	WORD_EOS WordId = 2 // Vocab always uses this for </s>.
)

// StateId represents a Model state.
type StateId uint32

const (
	STATE_NIL    StateId = ^StateId(0) // An invalid state.
	_STATE_EMPTY StateId = 0           // Model always uses state 0 for empty context.
	_STATE_START StateId = 1           // Model always uses state 1 for start.
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
