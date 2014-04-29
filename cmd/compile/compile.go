package main

import (
	"encoding/gob"
	"flag"
	"github.com/golang/glog"
	"github.com/kho/fslm"
	"os"
)

func main() {
	flag.Parse()
	flag.Set("logtostderr", "true")

	model, err := fslm.FromARPA(os.Stdin)
	if err != nil {
		glog.Fatal(err)
	}
	if err := gob.NewEncoder(os.Stdout).Encode(*model); err != nil {
		glog.Fatal(err)
	}
}
