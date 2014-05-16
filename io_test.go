package fslm

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func TestFromARPAFile(t *testing.T) {
	for _, i := range []string{"simple.3gram.arpa", "messy.3gram.arpa.gz"} {
		builder, err := FromARPAFile(path.Join("testdata", i))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		sentTest(builder.DumpHashed(0), simpleTrigramSents, t)
	}
}

func TestHashedBinary(t *testing.T) {
	model := readyBuilder(simpleTrigramLM).DumpHashed(0)

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

	kind, modelI, backing, err := FromBinary(path)
	if err != nil {
		t.Fatalf("error in loading binary: %v", err)
	}

	if kind != MODEL_HASHED {
		t.Fatalf("expect kind %d; got %d", MODEL_HASHED, kind)
	}

	sentTest(modelI.(*Hashed), simpleTrigramSents, t)

	modelI = nil
	if err := backing.Close(); err != nil {
		t.Errorf("error in closing mapped file: %v", err)
	}
}

func TestSortedBinary(t *testing.T) {
	model := readyBuilder(simpleTrigramLM).DumpSorted()

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

	kind, modelI, backing, err := FromBinary(path)
	if err != nil {
		t.Fatalf("error in loading binary: %v", err)
	}

	if kind != MODEL_SORTED {
		t.Fatalf("expect kind %d; got %d", MODEL_SORTED, kind)
	}

	sentTest(modelI.(*Sorted), simpleTrigramSents, t)

	modelI = nil
	if err := backing.Close(); err != nil {
		t.Errorf("error in closing mapped file: %v", err)
	}
}
