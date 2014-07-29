package main

import (
	"flag"
	"os"
	"runtime/pprof"

	"github.com/golang/glog"
	"github.com/kho/easy"
	"github.com/kho/fslm"
)

type CanWriteBinary interface {
	WriteBinary(string) error
}

func main() {
	var args struct {
		Out string `name:"out" usage:"output path"`
	}
	cpuprofile := flag.String("cpuprofile", "", "path to write CPU profile")
	memprofile := flag.String("memprofile", "", "path to write memory profile")
	format := easy.StringChoice("fslm.format", "hash", "output format", []string{"hash", "sort"})
	scale := flag.Float64("fslm.scale", 1.5, "scale multiplier for deciding the hash table size; only active in hash format")
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

	builder, err := fslm.FromARPA(os.Stdin)
	if err != nil {
		glog.Fatal(err)
	}

	var model CanWriteBinary

	switch *format {
	case "hash":
		model = builder.DumpHashed(*scale)
	case "sort":
		model = builder.DumpSorted()
	default:
		glog.Fatalf("unknown format %q", *format)
	}

	if err := model.WriteBinary(args.Out); err != nil {
		glog.Fatal(err)
	}
}
