package fslm

// ARPA file parsing routine using iteratees.

import (
	"bytes"
	"fmt"
	"github.com/kho/stream"
	"log"
	"strconv"
)

// arpaTop is the top-level iteratee for parsing a complete ARPA file.
type arpaTop struct {
	builder *Builder
}

func (it arpaTop) Final() error { return stream.Match(`\data\`).Final() }
func (it arpaTop) Next(line []byte) (stream.Iteratee, bool, error) {
	return stream.Seq{
		stream.Match(`\data\`),
		skipNgramCounts{},
		stream.Star{ngramSection{it.builder}},
		stream.Match(`\end\`),
		stream.EOF}, false, nil
}

// skipNgramCounts skips the n-gram-count section.
type skipNgramCounts struct{}

func (_ skipNgramCounts) Final() error { return nil }
func (it skipNgramCounts) Next(line []byte) (stream.Iteratee, bool, error) {
	if line[0] == '\\' {
		return nil, false, nil
	}
	return it, true, nil
}

// ngramSection goes through one n-gram section and add all the n-gram
// entries to the builder.
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
	return newNgramEntries(n, it.builder), true, nil
}

// ngramEntries scans 0 or more n-gram entries of the given order and
// add them to the builder.
type ngramEntries struct {
	builder *Builder
	n       int
	// These are for avoiding repeated space allocation.
	p, bow  Weight
	context []string
	word    string
}

// newNgramEntries constructs a new ngramEntries with properly
// initialized stub data.
func newNgramEntries(n int, b *Builder) *ngramEntries {
	return &ngramEntries{b, n, 0, 0, make([]string, n-1), ""}
}

func (it *ngramEntries) Final() error { return nil }
func (it *ngramEntries) Next(line []byte) (stream.Iteratee, bool, error) {
	if line[0] == '\\' {
		log.Printf("%d-gram done", it.n)
		return nil, false, nil
	}
	if err := it.setParts(line); err != nil {
		return nil, false, err
	}
	it.builder.AddNgram(it.context, it.word, it.p, it.bow)
	return it, true, nil
}

func (it *ngramEntries) setParts(line []byte) error {
	// p
	x, xs := tokenSplit(line)
	if x == "" {
		return stream.ErrExpect("log-probability")
	}
	if f, err := strconv.ParseFloat(x, WEIGHT_SIZE); err != nil {
		return err
	} else {
		it.p = Weight(f)
	}
	// context
	for i := 1; i < it.n; i++ {
		x, xs = tokenSplit(xs)
		if x == "" {
			return stream.ErrExpect(fmt.Sprintf("%d context word(s)", it.n))
		}
		it.context[i-1] = x
	}
	// word
	x, xs = tokenSplit(xs)
	if x == "" {
		return stream.ErrExpect("word")
	}
	it.word = x
	// bow
	x, xs = tokenSplit(xs)
	if x == "" {
		it.bow = 0
	} else if f, err := strconv.ParseFloat(x, WEIGHT_SIZE); err == nil {
		it.bow = Weight(f)
	} else {
		return err
	}
	// no extra stuff
	if len(xs) != 0 {
		return stream.ErrExpect("end of line")
	}
	return nil
}

// Low-level lexer code.

func isSpace(b byte) bool {
	switch b {
	case '\t', '\v', '\f', '\r', ' ':
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
