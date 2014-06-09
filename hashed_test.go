package fslm

import (
	"bytes"
	"testing"
)

func TestHashedSimple(t *testing.T) {
	hashedTest(simpleTrigramLM, simpleTrigramSents, t)
}

func TestHashedSparse(t *testing.T) {
	hashedTest(sparseFivegramLM, sparseFivegramSents, t)
}

func TestHashedSparser(t *testing.T) {
	hashedTest(sparserFivegramLM, sparserFivegramSents, t)
}

func TestHashedTrickyBackOff(t *testing.T) {
	hashedTest(trickyBackOffLM, trickyBackOffSents, t)
}

func hashedTest(lm []ngram, sents [][]token, t *testing.T) {
	builder := readyBuilder(lm)

	var buf bytes.Buffer
	buf.WriteString("builder LM:\n")
	builder.Graphviz(&buf)
	model := builder.DumpHashed(0)

	buf.WriteString("model LM:\n")
	Graphviz(model, &buf)
	t.Log(buf.String())

	if err := checkModel(model); err != nil {
		t.Errorf("check model failed with error %v", err)
	}

	sentTest(model, sents, t)
}
