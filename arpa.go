package fslm

import (
	"bytes"
	"github.com/kho/easy"
	"github.com/kho/stream"
	"io"
	"strconv"
)

func FromARPA(in io.Reader) (*Model, error) {
	builder := NewBuilder(nil)
	if err := stream.Run(stream.EnumRead(in, lineSplit), arpaTop{builder}); err != nil {
		return nil, err
	}
	return builder.Dump(), nil
}

func FromARPAFile(path string) (*Model, error) {
	in, err := easy.Open(path)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	return FromARPA(in)
}

type arpaTop struct {
	builder *Builder
}

func (it arpaTop) Final() error { return stream.Match(`\data\`).Final() }
func (it arpaTop) Next(line []byte) (stream.Iteratee, bool, error) {
	return stream.Seq{
		stream.Match(`\data\`),
		skipNGramCounts{},
		stream.Star{ngramSection{it.builder}},
		stream.Match(`\end\`),
		stream.EOF}, false, nil
}

type skipNGramCounts struct{}

func (_ skipNGramCounts) Final() error { return nil }
func (it skipNGramCounts) Next(line []byte) (stream.Iteratee, bool, error) {
	if line[0] == '\\' {
		return nil, false, nil
	}
	return it, true, nil
}

type ngramSection struct {
	builder *Builder
}

func (it ngramSection) Final() error { return stream.ErrExpect(`\N-grams: ...`) }
func (it ngramSection) Next(line []byte) (stream.Iteratee, bool, error) {
	if line[0] != '\\' || !bytes.HasSuffix(line, []byte("-grams:")) {
		return nil, false, stream.ErrExpect(`section header "\N-grams:"`)
	}
	n, err := strconv.Atoi(string(line[1 : len(line)-len("-grams:")]))
	if err != nil || n <= 0 {
		return nil, false, stream.ErrExpect(`positive integer in section header "\N-grams:"`)
	}
	return newNgramWeights(n, it.builder), true, nil
}

type ngramWeights struct {
	builder *Builder
	n       int
	p, bow  Weight
	context []string
	word    string
}

func newNgramWeights(n int, b *Builder) *ngramWeights {
	return &ngramWeights{b, n, 0, 0, make([]string, n-1), ""}
}

func (it *ngramWeights) Final() error { return nil }
func (it *ngramWeights) Next(line []byte) (stream.Iteratee, bool, error) {
	if line[0] == '\\' {
		return nil, false, nil
	}
	if err := it.setParts(line); err != nil {
		return nil, false, err
	}
	it.builder.AddNGram(it.context, it.word, it.p, it.bow)
	return it, true, nil
}

func (it *ngramWeights) setParts(line []byte) error {
	// p
	x, xs := tokenSplit(line)
	if x == "" {
		goto fail
	}
	if f, err := strconv.ParseFloat(x, WEIGHT_SIZE); err != nil {
		goto fail
	} else {
		it.p = Weight(f)
	}
	// context
	for i := 1; i < it.n; i++ {
		x, xs = tokenSplit(xs)
		if x == "" {
			goto fail
		}
		it.context[i-1] = x
	}
	// word
	x, xs = tokenSplit(xs)
	if x == "" {
		goto fail
	}
	it.word = x
	// bow
	x, xs = tokenSplit(xs)
	if x == "" {
		it.bow = 0
	} else if f, err := strconv.ParseFloat(x, WEIGHT_SIZE); err == nil {
		it.bow = Weight(f)
	} else {
		goto fail
	}
	// no extra stuff
	if len(xs) != 0 {
		goto fail
	}
	return nil
fail:
	return stream.ErrExpect(`n-gram entry like "p    w1 ... wn    [bow]"`)
}

func isSpace(b byte) bool {
	switch b {
	case '\t', '\v', '\f', '\r', ' ', '\x85', '\xa0':
		return true
	default:
		return false
	}
}

func lineSplit(data []byte, atEOF bool) (int, []byte, error) {
	l, r, n := -1, -1, 0
	// Skip leading spaces or newlines.
	for i, b := range data {
		if !isSpace(b) && b != '\n' {
			l = i
			break
		}
	}
	if l < 0 {
		return len(data), nil, nil
	}
	// Find newline.
	for i, b := range data[l+1:] {
		if b == '\n' {
			r, n = l+i, l+i+2
			break
		}
	}
	if r < 0 {
		if !atEOF {
			return l, nil, nil
		}
		r, n = len(data)-1, len(data)
	}
	// Trim trailing spaces.
	for isSpace(data[r]) {
		// At most we shall stop at l.
		r--
	}
	return n, data[l : r+1], nil
}

func tokenSplit(line []byte) (string, []byte) {
	// Assuming line has no leading space.
	r := -1
	for i, b := range line {
		if isSpace(b) {
			r = i
			break
		}
	}
	if r < 0 {
		r = len(line)
	}
	token := string(line[:r])
	// Skip trailing spaces.
	for i, b := range line[r:] {
		if !isSpace(b) {
			return token, line[r+i:]
		}
	}
	return token, nil
}
