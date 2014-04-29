package main

import (
	"encoding/gob"
	"flag"
	"github.com/kho/fslm"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
	model, err := fslm.FromARPA(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	if err := gob.NewEncoder(os.Stdout).Encode(*model); err != nil {
		log.Fatal(err)
	}
}
