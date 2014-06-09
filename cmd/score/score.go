package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/kho/easy"
	"github.com/kho/fslm"
	"github.com/kho/word"
	"io"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

var unkScore fslm.Weight

func init() {
	flag.Var(&unkScore, "unk", "score for <unk>")
}

func main() {
	var args struct {
		Model string `name:"model" usage:"LM file"`
	}
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

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	kind, modelI, file, err := fslm.FromBinary(args.Model)
	if err != nil {
		glog.Fatal("error in loading model: ", err)
	}
	defer file.Close()
	runtime.GC()
	runtime.ReadMemStats(&after)
	glog.Infof("LM memory overhead: %.2fMB", float64(after.Alloc-before.Alloc)/float64(1<<20))

	var (
		corpus                      [][]word.Id
		score                       float64
		numWords, numSents, numOOVs int
	)

	glog.Info("loading corpus took ", easy.Timed(func() { corpus = LoadCorpus(os.Stdin, modelI) }))

	numSents = len(corpus)
	for _, i := range corpus {
		numWords += len(i)
	}

	elapsed := easy.Timed(func() {
		score, numOOVs = ScoreCorpus(kind, modelI, corpus)
	})
	glog.Infof("scoring took %v; %g QPS", elapsed, float64(numSents+numWords)*float64(time.Second)/float64(elapsed))

	if numWords > 0 {
		fmt.Printf("%d sents, %d words, %d OOVs\n", numSents, numWords, numOOVs)
		fmt.Printf("logprob=%g ppl=%g ppl1=%g\n",
			score, math.Exp(-float64(score)/float64(numSents+numWords)*math.Log(10)),
			math.Exp(-float64(score)/float64(numWords)*math.Log(10)))
	}
}

func LoadCorpus(r io.Reader, modelI interface{}) (sents [][]word.Id) {
	in := bufio.NewScanner(r)
	vocab, _, _, _, _ := modelI.(fslm.Model).Vocab()
	for in.Scan() {
		var sent []word.Id
		for _, i := range bytes.Fields(in.Bytes()) {
			sent = append(sent, vocab.IdOf(string(i)))
		}
		sents = append(sents, sent)
	}
	if err := in.Err(); err != nil {
		glog.Fatal("when loading corpus: ", err)
	}
	return
}

func ScoreCorpus(kind int, modelI interface{}, corpus [][]word.Id) (score float64, numOOVs int) {
	if glog.V(1) {
		return VerboseScoreCorpus(modelI.(fslm.Model), corpus)
	} else {
		switch kind {
		case fslm.MODEL_HASHED:
			return SilentScoreCorpusHashed(modelI.(*fslm.Hashed), corpus)
		case fslm.MODEL_SORTED:
			return SilentScoreCorpusSorted(modelI.(*fslm.Sorted), corpus)
		}
	}
	return
}

func VerboseScoreCorpus(model fslm.Model, corpus [][]word.Id) (total float64, numOOVs int) {
	for _, sent := range corpus {
		p := model.Start()
		for _, x := range sent {
			var w fslm.Weight
			p, w = model.NextI(p, x)
			if w == fslm.WEIGHT_LOG0 {
				w = unkScore
				numOOVs++
				fmt.Printf("<unk>")
			} else {
				fmt.Printf("%q", x)
			}
			total += float64(w)
			fmt.Printf("\t%g\t%g\n", w, total)
		}
		w := model.Final(p)
		total += float64(w)
		fmt.Printf("</s>\t%g\t%g\n\n", w, total)
	}
	return
}

func SilentScoreCorpusHashed(model *fslm.Hashed, corpus [][]word.Id) (total float64, numOOVs int) {
	for _, sent := range corpus {
		p := model.Start()
		for _, x := range sent {
			var w fslm.Weight
			p, w = model.NextI(p, x)
			if w == fslm.WEIGHT_LOG0 {
				w = unkScore
				numOOVs++
			}
			total += float64(w)
		}
		w := model.Final(p)
		total += float64(w)
	}
	return
}

func SilentScoreCorpusSorted(model *fslm.Sorted, corpus [][]word.Id) (total float64, numOOVs int) {
	for _, sent := range corpus {
		p := model.Start()
		for _, x := range sent {
			var w fslm.Weight
			p, w = model.NextI(p, x)
			if w == fslm.WEIGHT_LOG0 {
				w = unkScore
				numOOVs++
			}
			total += float64(w)
		}
		w := model.Final(p)
		total += float64(w)
	}
	return
}
