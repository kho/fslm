package main

import (
	"encoding/gob"
	"github.com/golang/glog"
	"github.com/kho/easy"
	"github.com/kho/fslm"
	"os"
)

func main() {
	easy.ParseFlagsAndArgs(nil)

	model, err := fslm.FromARPA(os.Stdin)
	if err != nil {
		glog.Fatal(err)
	}
	if err := gob.NewEncoder(os.Stdout).Encode(*model); err != nil {
		glog.Fatal(err)
	}
}
