package fslm

import (
	"testing"
)

func TestVocab(t *testing.T) {
	v := NewVocab([]string{"<unk>", "<s>", "</s>"})

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
		{"<unk>", 0}, {"<s>", 1}, {"</s>", 2}, {"x", x}, {"y", WORD_NIL},
	} {
		if a := v.IdOf(i.S); a != i.I {
			t.Errorf("expected v.IdOf(%q) = %d; got %d", i.S, i.I, a)
		}
		if i.I != WORD_NIL {
			if a := v.StringOf(i.I); a != i.S {
				t.Errorf("expected v.StringOf(%d) = %q; got %q", i.I, i.S, a)
			}
		}
	}

	if b := v1.IdOf("b"); b != WORD_NIL {
		t.Errorf("expected v1.IdOf(%q) = %d; got %d", "b", WORD_NIL, b)
	}
	if a := v2.IdOf("a"); a != WORD_NIL {
		t.Errorf("expected v2.IdOf(%q) = %d; got %d", "b", WORD_NIL, a)
	}
	if a := v.IdOf("a"); a != WORD_NIL {
		t.Errorf("expected v.IdOf(%q) = %d; got %d", "a", WORD_NIL, a)
	}
	if b := v.IdOf("b"); b != WORD_NIL {
		t.Errorf("expected v.IdOf(%q) = %d; got %d", "a", WORD_NIL, b)
	}

	v.IdOrAdd("y")
	if y := v1.IdOf("y"); y != WORD_NIL {
		t.Errorf("expected v1.IdOf(%q) = %d; got %d", "y", WORD_NIL, y)
	}
	if y := v2.IdOf("y"); y != WORD_NIL {
		t.Errorf("expected v2.IdOf(%q) = %d; got %d", "y", WORD_NIL, y)
	}

	y := v.IdOf("y")
	if yy := v.IdOrAdd("y"); yy != y {
		t.Errorf("expected v.IdOrAdd(%q) = %d; got %d", "y", y, yy)
	}

	if b := v.Bound(); b != 5 {
		t.Errorf("expected v.Bound() = 5; got %d", b)
	}

	for _, i := range [][]string{
		{"a", "a", "c"}, {"a", "b", "a"}, {"a", "b", "b"},
	} {
		func() {
			defer func() {
				err := recover()
				if err == nil {
					t.Error("expected panic; got nil error")
				}
			}()
			NewVocab(i)
		}()
	}
}
