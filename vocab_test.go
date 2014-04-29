package fslm

import (
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
