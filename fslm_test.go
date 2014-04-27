package fslm

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestVocab(t *testing.T) {
	v := NewVocab("<unk>", "<s>", "</s>")

	if b := v.Bound(); b != 3 {
		t.Errorf("expected v.Bound() = 3; got %d", b)
	}

	x := v.IdOrAdd("x")
	v1, v2 := v.Copy(), v.Copy()
	v1.IdOrAdd("a")
	v2.IdOrAdd("b")

	for _, i := range []struct {
		S string
		I WordId
	}{
		{"<unk>", WORD_UNK}, {"<s>", WORD_BOS}, {"</s>", WORD_EOS}, {"x", x}, {"y", WORD_UNK},
	} {
		if a := v.IdOf(i.S); a != i.I {
			t.Errorf("expected v.IdOf(%q) = %d; got %d", i.S, i.I, a)
		}
		if i.I == WORD_UNK && i.S != "<unk>" {
			i.S = "<unk>"
		}
		if a := v.StringOf(i.I); a != i.S {
			t.Errorf("expected v.StringOf(%d) = %q; got %q", i.I, i.S, a)
		}
	}

	if b := v1.IdOf("b"); b != WORD_UNK {
		t.Errorf("expected v1.IdOf(%q) = %d; got %d", "b", WORD_UNK, b)
	}
	if a := v2.IdOf("a"); a != WORD_UNK {
		t.Errorf("expected v2.IdOf(%q) = %d; got %d", "b", WORD_UNK, a)
	}
	if a := v.IdOf("a"); a != WORD_UNK {
		t.Errorf("expected v.IdOf(%q) = %d; got %d", "a", WORD_UNK, a)
	}
	if b := v.IdOf("b"); b != WORD_UNK {
		t.Errorf("expected v.IdOf(%q) = %d; got %d", "a", WORD_UNK, b)
	}

	v.IdOrAdd("y")
	if y := v1.IdOf("y"); y != WORD_UNK {
		t.Errorf("expected v1.IdOf(%q) = %d; got %d", "y", WORD_UNK, y)
	}
	if y := v2.IdOf("y"); y != WORD_UNK {
		t.Errorf("expected v2.IdOf(%q) = %d; got %d", "y", WORD_UNK, y)
	}

	y := v.IdOf("y")
	if yy := v.IdOrAdd("y"); yy != y {
		t.Errorf("expected v.IdOrAdd(%q) = %d; got %d", "y", y, yy)
	}

	if b := v.Bound(); b != 5 {
		t.Errorf("expected v.Bound() = 5; got %d", b)
	}

	for _, i := range [][3]string{
		{"a", "a", "c"}, {"a", "b", "a"}, {"a", "b", "b"},
	} {
		func() {
			defer func() {
				err := recover()
				if err == nil {
					t.Error("expected panic; got nil error")
				}
			}()
			NewVocab(i[0], i[1], i[2])
		}()
	}
}

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
	{"", "</s>", 0.1, 0},
	{"a b c", "d", -1, -2},
	{"b c", "e", -4, 1},
	{"c", "d", 0, -3},
}

var trickyBackOffSents = [][]token{
	{{"a", 0}, {"b", 0}, {"c", 0}, {"d", -1}, {"</s>", -2 - 3 + 0.1}},
	{{"a", 0}, {"b", 0}, {"c", 0}, {"e", -4}, {"</s>", 1 + 0.1}},
}

func lmTest(lm []ngram, sents [][]token, t *testing.T) {
	builder := NewBuilder(nil)
	for _, i := range lm {
		c, x, w, b := i.Params()
		builder.AddNGram(c, x, w, b)
	}
	model := builder.Dump()

	if err := checkModel(model); err != nil {
		t.Errorf("check model failed with error %v", err)
	}

	var buf bytes.Buffer
	model.Graphviz(&buf)
	t.Log("LM:\n", buf.String())

	for _, i := range sents {
		p := model.Start()
		w := Weight(99)
		for j, x := range i {
			if x.Word != "</s>" {
				p, w = model.NextS(p, x.Word)
			} else {
				w = model.Final(p)
			}
			if w != x.Weight {
				t.Errorf("expected weight = %g; got %g\nsent: %v\nword: %d@%v", x.Weight, w, i, j, x)
			}
		}
	}
}

func checkModel(m *Model) error {
	// All states should be reachable from _STATE_START.
	uf := newUnionFind(len(m.states))
	for i, s := range m.states {
		if s.BackOffState != STATE_NIL {
			uf.Union(i, int(s.BackOffState))
		}
	}
	for px, qw := range m.transitions {
		if qw.Tgt != STATE_NIL {
			uf.Union(int(px.Src()), int(qw.Tgt))
		}
	}
	for i := range uf {
		if uf.Find(i) != uf.Find(int(_STATE_START)) {
			return errors.New("there are non-reachable states")
		}
	}
	// _STATE_EMPTY backs off to STATE_NIL.
	if m.states[_STATE_EMPTY].BackOffState != STATE_NIL {
		return errors.New("wrong back-off for _STATE_EMPTY")
	}
	// All other states eventually backs off to _STATE_EMPTY.
	uf = newUnionFind(len(m.states))
	for i, s := range m.states {
		if s.BackOffState != STATE_NIL {
			uf.Union(int(s.BackOffState), i)
		}
	}
	for i := range uf[_STATE_START:] {
		if uf.Find(i) != int(_STATE_EMPTY) {
			return errors.New("there are states that do not back off to empty")
		}
	}
	// Every back-off state has at least one lexical transition.
	internal := map[StateId]bool{}
	for px, _ := range m.transitions {
		internal[px.Src()] = true
	}
	for _, s := range m.states[_STATE_START:] {
		if !internal[s.BackOffState] {
			return errors.New("backing off to leaf state")
		}
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

func TestLMs(t *testing.T) {
	lmTest(simpleTrigramLM, simpleTrigramSents, t)
	lmTest(sparseFivegramLM, sparseFivegramSents, t)
	lmTest(sparserFivegramLM, sparserFivegramSents, t)
	lmTest(trickyBackOffLM, trickyBackOffSents, t)
}
