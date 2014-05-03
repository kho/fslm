package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/kho/easy"
	"github.com/kho/fslm"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
)

var unkScore fslm.Weight

func init() {
	flag.Var(&unkScore, "unk", "score for <unk>")
}

func main() {
	var args struct {
		Model string `name:"model" usage:"LM file"`
	}
	format := flag.String("format", "bin", "arpa or gob")
	cpuprofile := flag.String("cpuprofile", "", "path to write CPU profile")
	memprofile := flag.String("memprofile", "", "path to write memory profile")
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

	var (
		model         *fslm.Model
		err           error
		before, after runtime.MemStats
	)
	runtime.GC()
	runtime.ReadMemStats(&before)
	switch *format {
	case "arpa":
		model, err = fslm.FromARPAFile(args.Model, 0)
	case "bin":
		var mappedFile *fslm.MappedFile
		model, mappedFile, err = fslm.FromBinary(args.Model)
		defer mappedFile.Close()
	case "gob":
		model, err = fslm.FromGobFile(args.Model)
	default:
		glog.Fatalf("unknown format %q", format)
	}
	if err != nil {
		glog.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	numStates, numTransitions, numWords := model.Size()
	glog.Infof("loaded LM with %d states, %d transitions, and %d words", numStates, numTransitions, numWords)
	glog.Infof("LM memory usage: %.2fMB", float64(after.Alloc-before.Alloc)/float64(1<<20))
	in := bufio.NewScanner(os.Stdin)

	score, numWords, numSents, numOOVs := fslm.Weight(0), 0, 0, 0
	if glog.V(1) {
		for in.Scan() {
			sent := strings.Fields(in.Text())
			s, o := Score(model, sent)
			score += s
			numWords += len(sent)
			numSents++
			numOOVs += o
		}
	} else {
		for in.Scan() {
			sent := strings.Fields(in.Text())
			s, o := SilentScore(model, sent)
			score += s
			numWords += len(sent)
			numSents++
			numOOVs += o
		}
	}
	if err := in.Err(); err != nil {
		glog.Fatal(err)
	}

	if numWords > 0 {
		fmt.Printf("%d sents, %d words, %d OOVs\n", numSents, numWords, numOOVs)
		fmt.Printf("logprob=%g ppl=%g ppl1=%g\n",
			score, math.Exp(-float64(score)/float64(numSents+numWords)*math.Log(10)),
			math.Exp(-float64(score)/float64(numWords)*math.Log(10)))
	}
}

func Score(model *fslm.Model, sent []string) (total fslm.Weight, numOOVs int) {
	p := model.Start()
	for _, x := range sent {
		var w fslm.Weight
		p, w = model.NextS(p, x)
		if w == fslm.WEIGHT_LOG0 {
			w = unkScore
			numOOVs++
			fmt.Printf("<unk>")
		} else {
			fmt.Printf("%q", x)
		}
		total += w
		fmt.Printf("\t%g\t%g\n", w, total)
	}
	w := model.Final(p)
	total += w
	fmt.Printf("</s>\t%g\t%g\n\n", w, total)
	return
}

func SilentScore(model *fslm.Model, sent []string) (total fslm.Weight, numOOVs int) {
	p := model.Start()
	for _, x := range sent {
		var w fslm.Weight
		p, w = model.NextS(p, x)
		if w == fslm.WEIGHT_LOG0 {
			w = unkScore
			numOOVs++
		}
		total += w
	}
	w := model.Final(p)
	total += w
	return
}
