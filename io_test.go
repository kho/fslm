package fslm

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func TestFromARPAFile(t *testing.T) {
	for _, i := range []string{"simple.3gram.arpa", "messy.3gram.arpa.gz"} {
		model, err := FromARPAFile(path.Join("testdata", i), 0)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		sentTest(model, simpleTrigramSents, t)
	}
}

func TestBinary(t *testing.T) {
	model, err := FromARPAFile(path.Join("testdata", "simple.3gram.arpa"), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f, err := ioutil.TempFile("", "binary.")
	if err != nil {
		t.Fatalf("error in creating temporary file: %v", err)
	}
	path := f.Name()
	f.Close()
	defer func() {
		os.Remove(path)
	}()

	if err := model.WriteBinary(path); err != nil {
		t.Fatalf("error in writing binary: %v", err)
	}

	model, backing, err := FromBinary(path)
	if err != nil {
		t.Fatalf("error in loading binary: %v", err)
	}

	sentTest(model, simpleTrigramSents, t)

	model = nil
	if err := backing.Close(); err != nil {
		t.Errorf("error in closing mapped file: %v", err)
	}
}
