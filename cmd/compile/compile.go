package main

import (
	"flag"
	"github.com/golang/glog"
	"github.com/kho/easy"
	"github.com/kho/fslm"
	"os"
	"runtime/pprof"
)

func main() {
	var args struct {
		Out string `name:"out" usage:"output path"`
	}
	cpuprofile := flag.String("cpuprofile", "", "path to write CPU profile")
	memprofile := flag.String("memprofile", "", "path to write memory profile")
	scale := flag.Float64("fslm.scale", 1.5, "scale multiplier for deciding the hash table size")
	easy.ParseFlagsAndArgs(&args)

	if *cpuprofile != "" {
		w := easy.MustCreate(*cpuprofile)
		pprof.StartCPUProfile(w)
		defer func() {
			pprof.StopCPUProfile()
			w.Close()
		}()
	}

	if *memprofile != "" {
		defer func() {
			w := easy.MustCreate(*memprofile)
			pprof.WriteHeapProfile(w)
			w.Close()
		}()
	}

	model, err := fslm.FromARPA(os.Stdin, *scale)
	if err != nil {
		glog.Fatal(err)
	}
	if err := model.WriteBinary(args.Out); err != nil {
		glog.Fatal(err)
	}
}
