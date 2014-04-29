package main

import (
	"encoding/gob"
	"flag"
	"github.com/golang/glog"
	"github.com/kho/easy"
	"github.com/kho/fslm"
	"os"
)

func main() {
	scale := flag.Float64("fslm.scale", 1.5, "scale multiplier for deciding the hash table size")
	easy.ParseFlagsAndArgs(nil)

	model, err := fslm.FromARPA(os.Stdin, *scale)
	if err != nil {
		glog.Fatal(err)
	}
	if err := gob.NewEncoder(os.Stdout).Encode(*model); err != nil {
		glog.Fatal(err)
	}
}
