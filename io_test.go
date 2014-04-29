package fslm

import (
	"path"
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
