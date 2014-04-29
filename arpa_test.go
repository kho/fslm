package fslm

import (
	"bufio"
	"path"
	"reflect"
	"strings"
	"testing"
)

func TestFromARPAFile(t *testing.T) {
	for _, i := range []string{"simple.3gram.arpa", "messy.3gram.arpa.gz"} {
		model, err := FromARPAFile(path.Join("testdata", i))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		sentTest(model, simpleTrigramSents, t)
	}
}

func Test_lineSplit(t *testing.T) {
	for _, i := range []struct {
		Data  string
		Lines []string
	}{
		{"a\nb\n", []string{"a", "b"}},
		{"ab\ncd", []string{"ab", "cd"}},
		{" \tab\ncd \n", []string{"ab", "cd"}},
		{"\nab\n\ncd\n\n", []string{"ab", "cd"}},
		{"", nil},
		{"\n\n\n\n", nil},
	} {
		in := bufio.NewScanner(strings.NewReader(i.Data))
		in.Split(lineSplit)
		var lines []string
		for in.Scan() {
			lines = append(lines, in.Text())
		}
		if err := in.Err(); err != nil {
			t.Errorf("case %q: unexpected error: %v", i.Data, err)
		}
		if len(lines) != len(i.Lines) {
			t.Errorf("case %q: expect %d lines; got %q", i.Data, len(i.Lines), lines)
		} else {
			for j, l := range i.Lines {
				if l != lines[j] {
					t.Errorf("case %q: expect %q as line %d; got %q", i.Data, l, j+1, lines[j])
				}
			}
		}
	}
}

func Test_tokenSplit(t *testing.T) {
	for _, i := range []struct {
		Line   string
		Tokens []string
	}{
		{"a b c", []string{"a", "b", "c"}},
		{"ab cd", []string{"ab", "cd"}},
		{"", nil},
		{"ab \t cd", []string{"ab", "cd"}},
		{"ab cd \t ", []string{"ab", "cd"}},
	} {
		var tokens []string
		for x, xs := tokenSplit([]byte(i.Line)); x != ""; x, xs = tokenSplit(xs) {
			tokens = append(tokens, x)
		}
		if len(i.Tokens) != len(tokens) {
			t.Errorf("case %q: expect %d tokens; got %q", i.Line, len(i.Tokens), tokens)
		} else {
			for j, a := range i.Tokens {
				if a != tokens[j] {
					t.Errorf("case %q: expect %q as token %d; got %q", i.Line, a, j+1, tokens[j])
				}
			}
		}
	}
}

func Test_ngramWeights_setParts(t *testing.T) {
	for _, i := range []struct {
		N       int
		Line    string
		Err     bool
		P, BOW  Weight
		Context []string
		Word    string
	}{
		{1, "-1 a -2", false, -1, -2, nil, "a"},
		{1, "-1 ab", false, -1, 0, nil, "ab"},
		{2, "-1 ab cd -2", false, -1, -2, []string{"ab"}, "cd"},
		{6, "-3 ab cd ef gh ij kl", false, -3, 0, []string{"ab", "cd", "ef", "gh", "ij"}, "kl"},
		{1, "-1 -2", false, -1, 0, nil, "-2"},
		{4, "-1 -2 -3 -4 -5", false, -1, 0, []string{"-2", "-3", "-4"}, "-5"},
		{3, "-1 -2 -3 -4 -5", false, -1, -5, []string{"-2", "-3"}, "-4"},
		{N: 3, Line: "-1 ab cd", Err: true},
		{N: 1, Line: "", Err: true},
		{N: 2, Line: "-1", Err: true},
		{N: 2, Line: "-1 ab cd -4 -5", Err: true},
		{N: 2, Line: "ab cd ef", Err: true},
		{N: 2, Line: "-1 ab cd ef", Err: true},
	} {
		nw := newNgramWeights(i.N, nil)
		// Mess up the state before setting.
		nw.p = 9999
		nw.bow = 9999
		for j := 1; j < i.N; j++ {
			nw.context[j-1] = "haha"
		}
		nw.word = "hoho"
		err := nw.setParts([]byte(i.Line))
		if i.Err && err == nil {
			t.Errorf("case %+v: expect error", i)
		}
		if !i.Err && err != nil {
			t.Errorf("case %+v: unexpected error: %v", i, err)
		}
		if err == nil {
			if nw.p != i.P {
				t.Errorf("case %+v: nw.p = %g", i, nw.p)
			}
			if nw.bow != i.BOW {
				t.Errorf("case %+v: nw.bow = %g", i, nw.bow)
			}
			if len(nw.context) == 0 {
				nw.context = nil
			} // reflect.DeepEqual(nil, empty_slice) = false!
			if !reflect.DeepEqual(nw.context, i.Context) {
				t.Errorf("case %+v: nw.context = %q", i, nw.context)
			}
			if nw.word != i.Word {
				t.Errorf("case %+v: nw.word = %q", i, nw.word)
			}
		}
	}
}
