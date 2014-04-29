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
	"strings"
)

var (
	unkScore fslm.Weight
	format   string
)

func init() {
	flag.Var(&unkScore, "unk", "score for <unk>")
	flag.StringVar(&format, "format", "gob", "arpa or gob")
}

func main() {
	var args struct {
		Model string `name:"model" usage:"LM file"`
	}
	easy.ParseFlagsAndArgs(&args)

	var (
		model *fslm.Model
		err   error
	)
	switch format {
	case "arpa":
		model, err = fslm.FromARPAFile(args.Model)
	case "gob":
		model, err = fslm.FromGobFile(args.Model)
	default:
		glog.Fatalf("unknown format %q", format)
	}
	if err != nil {
		glog.Fatal(err)
	}
	numStates, numTransitions, numWords := model.Size()
	if glog.V(1) {
		glog.Infof("loaded LM with %d states, %d transitions, and %d words", numStates, numTransitions, numWords)
	}

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
		}
		total += w
		fmt.Printf("%q\t%g\t%g", x, w, total)
	}
	w := model.Final(p)
	total += w
	fmt.Printf("</s>\t%g\t%g\n", w, total)
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
