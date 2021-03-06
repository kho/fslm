package fslm

// Common routines for testing a language model.

import (
	"errors"
	"strings"
	"testing"
)

type ngram struct {
	Context, Word   string
	Weight, BackOff Weight
}

func (n ngram) Params() ([]string, string, Weight, Weight) {
	var context []string
	if n.Context != "" {
		context = strings.Fields(n.Context)
	}
	return context, n.Word, n.Weight, n.BackOff
}

type token struct {
	Word   string
	Weight Weight
}

var simpleTrigramLM = []ngram{
	{"", "<s>", WEIGHT_LOG0, -1},
	{"", "</s>", -0.01, 0},
	{"", "a", -2, -1},
	{"", "b", -4, -2},
	{"<s>", "a", -1, -0.5},
	{"a", "b", -2, -1},
	{"<s> a", "b", -1.5, 0},
	{"a b", "</s>", -0.001, 0},
}

var simpleTrigramSents = [][]token{
	{{"a", -1}, {"</s>", -0.5 - 1 - 0.01}},
	{{"a", -1}, {"b", -1.5}, {"</s>", -0.001}},
	{{"a", -1}, {"b", -1.5}, {"a", -1 - 2 - 2}, {"b", -2}, {"</s>", -0.001}},
	{{"a", -1}, {"b", -1.5}, {"c", WEIGHT_LOG0}, {"</s>", -0.01}},
}

var sparseFivegramLM = []ngram{
	{"", "<s>", WEIGHT_LOG0, -1},
	{"", "</s>", 0.1, 0},
	{"<s> a a a", "a", -1, -2},
	{"a a", "a", -3, -4},
}

var sparseFivegramSents = [][]token{
	{{"a", 0}, {"</s>", 0.1}},
	{{"a", 0}, {"a", 0}, {"</s>", 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"</s>", -4 + 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"a", -1}, {"</s>", -2 - 4 + 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"a", -1}, {"a", -2 - 4 - 3}, {"</s>", -4 + 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"a", -1}, {"a", -2 - 4 - 3}, {"a", -4 - 3}, {"</s>", -4 + 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"a", -1}, {"a", -2 - 4 - 3}, {"a", -4 - 3}, {"a", -4 - 3}, {"</s>", -4 + 0.1}},
}

var sparserFivegramLM = []ngram{
	{"", "<s>", WEIGHT_LOG0, -1},
	{"", "</s>", 0.1, 0},
	{"<s> a a a", "a", -1, -2},
}

var sparserFivegramSents = [][]token{
	{{"a", 0}, {"</s>", 0.1}},
	{{"a", 0}, {"a", 0}, {"</s>", 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"</s>", 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"a", -1}, {"</s>", -2 + 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"a", -1}, {"a", WEIGHT_LOG0}, {"</s>", 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"a", -1}, {"a", WEIGHT_LOG0}, {"a", WEIGHT_LOG0}, {"</s>", 0.1}},
	{{"a", 0}, {"a", 0}, {"a", 0}, {"a", -1}, {"a", WEIGHT_LOG0}, {"a", WEIGHT_LOG0}, {"a", WEIGHT_LOG0}, {"</s>", 0.1}},
}

var trickyBackOffLM = []ngram{
	{"", "<s>", 0, -1},
	{"", "</s>", 0.1, 0},
	{"a b c", "d", -1, -2},
	{"b c", "e", -4, 1},
	{"c", "d", 0, -3},
}

var trickyBackOffSents = [][]token{
	{{"</s>", -1 + 0.1}},
	{{"a", -1}, {"b", 0}, {"c", 0}, {"d", -1}, {"</s>", -2 - 3 + 0.1}},
	{{"a", -1}, {"b", 0}, {"c", 0}, {"e", -4}, {"</s>", 1 + 0.1}},
}

const floatTol = 1e-7

func readyBuilder(lm []ngram) *Builder {
	builder := NewBuilder(nil, "", "")
	for _, i := range lm {
		c, x, w, b := i.Params()
		builder.AddNgram(c, x, w, b)
	}
	return builder
}

func sentTest(model Model, sents [][]token, t *testing.T) {
	for _, i := range sents {
		var (
			w0, w1 Weight
			ws     []Weight
		)
		p := model.Start()
		for _, x := range i {
			var w Weight
			if x.Word != "</s>" {
				p, w = model.NextS(p, x.Word)
			} else {
				w = model.Final(p)
			}
			w0 += x.Weight
			w1 += w
			ws = append(ws, w)
		}
		if w0-w1 >= floatTol || w1-w0 >= floatTol {
			t.Errorf("expected total weight = %g; got %g\nsent: %v\nweights: %v", w0, w1, i, ws)
		}
	}
}

func checkModel(m IterableModel) error {
	// All states should be reachable from _STATE_START.
	uf := newUnionFind(m.NumStates())
	for i := 0; i < m.NumStates(); i++ {
		p := StateId(i)
		backoff, _ := m.BackOff(p)
		if backoff != STATE_NIL {
			uf.Union(i, int(backoff))
		}
		for xqw := range m.Transitions(p) {
			if xqw.State != STATE_NIL {
				uf.Union(int(p), int(xqw.State))
			}
		}
	}
	for i := range uf {
		if uf.Find(i) != uf.Find(int(_STATE_START)) {
			return errors.New("there are non-reachable states")
		}
	}
	// _STATE_EMPTY backs off to STATE_NIL.
	if p, _ := m.BackOff(_STATE_EMPTY); p != STATE_NIL {
		return errors.New("wrong back-off for _STATE_EMPTY")
	}
	// All other states eventually backs off to _STATE_EMPTY.
	uf = newUnionFind(m.NumStates())
	for i := 0; i < m.NumStates(); i++ {
		if b, _ := m.BackOff(StateId(i)); b != STATE_NIL {
			uf.Union(int(b), i)
		}
	}
	for i := range uf[_STATE_EMPTY+1:] {
		if uf.Find(i) != int(_STATE_EMPTY) {
			return errors.New("there are states that do not back off to empty")
		}
	}
	// Every back-off state has at least one lexical transition.
	internal := map[StateId]bool{}
	for i := 0; i < m.NumStates(); i++ {
		p := StateId(i)
		n := 0
		for _ = range m.Transitions(p) {
			n++
		}
		if n > 0 {
			internal[p] = true
		}
	}
	for i := int(_STATE_EMPTY + 1); i < m.NumStates(); i++ {
		b, _ := m.BackOff(StateId(i))
		if !internal[b] {
			return errors.New("backing off to leaf state")
		}
	}
	delete(internal, _STATE_START)
	if len(internal)+1 != m.NumStates() {
		return errors.New("there are non-start leaf states")
	}
	return nil
}

type unionFind []int

func newUnionFind(n int) unionFind {
	uf := make(unionFind, n)
	for i := range uf {
		uf[i] = i
	}
	return uf
}

func (uf unionFind) Union(a, b int) int {
	ra, rb := uf.Find(a), uf.Find(b)
	uf[rb] = ra
	return ra
}

func (uf unionFind) Find(a int) int {
	r := uf[a]
	for r != uf[r] {
		r = uf[r]
	}
	for uf[a] != r {
		uf[a], a = r, uf[a]
	}
	return r
}
