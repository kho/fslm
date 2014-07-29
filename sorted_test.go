package fslm

import (
	"bytes"
	"errors"
	"testing"

	"github.com/kho/word"
)

func TestSortedSimple(t *testing.T) {
	sortedTest(simpleTrigramLM, simpleTrigramSents, t)
}

func TestSortedSparse(t *testing.T) {
	sortedTest(sparseFivegramLM, sparseFivegramSents, t)
}

func TestSortedSparser(t *testing.T) {
	sortedTest(sparserFivegramLM, sparserFivegramSents, t)
}

func TestSortedTrickyBackOff(t *testing.T) {
	sortedTest(trickyBackOffLM, trickyBackOffSents, t)
}

func sortedTest(lm []ngram, sents [][]token, t *testing.T) {
	builder := readyBuilder(lm)

	var buf bytes.Buffer
	buf.WriteString("builder LM:\n")
	builder.Graphviz(&buf)
	model := builder.DumpSorted()

	buf.WriteString("model LM:\n")
	Graphviz(model, &buf)
	t.Log(buf.String())

	if err := checkSorted(model); err != nil {
		t.Errorf("check sorted model failed with error %v", err)
	}

	if err := checkModel(model); err != nil {
		t.Errorf("check model failed with error %v", err)
	}

	sentTest(model, sents, t)
}

func checkSorted(m *Sorted) error {
	// Every slice should be uniquely sorted and have back-off as the last
	// transition.
	for _, next := range m.transitions {
		if len(next) == 0 {
			return errors.New("empty slice")
		}
		if next[len(next)-1].Word != word.NIL {
			return errors.New("last transition is not back-off")
		}
		for i, cur := range next[1:] {
			prev := next[i]
			if prev.Word >= cur.Word {
				return errors.New("not uniquely sorted by word")
			}
		}
	}
	return nil
}
