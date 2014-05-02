package main

import (
	"flag"
	"github.com/golang/glog"
	"github.com/kho/easy"
	"github.com/kho/fslm"
	"os"
)

func main() {
	var args struct {
		Out string `name:"out" usage:"output path"`
	}
	scale := flag.Float64("fslm.scale", 1.5, "scale multiplier for deciding the hash table size")
	easy.ParseFlagsAndArgs(&args)

	model, err := fslm.FromARPA(os.Stdin, *scale)
	if err != nil {
		glog.Fatal(err)
	}
	if err := model.WriteBinary(args.Out); err != nil {
		glog.Fatal(err)
	}
}
