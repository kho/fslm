package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/kho/easy"
	"github.com/kho/fslm"
	"log"
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
	flag.StringVar(&format, "format", "arpa", "arpa or gob")
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
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
		log.Fatalf("unknown format %q", format)
	}
	if err != nil {
		log.Fatal(err)
	}
	numStates, numTransitions, numWords := model.Size()
	log.Printf("loaded LM with %d states, %d transitions, and %d words", numStates, numTransitions, numWords)

	in := bufio.NewScanner(os.Stdin)

	score, numWords, numSents, numOOVs := fslm.Weight(0), 0, 0, 0
	for in.Scan() {
		sent := strings.Fields(in.Text())
		s, o := Score(model, sent)
		score += s
		numWords += len(sent)
		numSents++
		numOOVs += o
	}
	if err := in.Err(); err != nil {
		log.Fatal(err)
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
			// fmt.Print("<unk>")
		} else {
			// fmt.Printf("%q", x)
		}
		total += w
		// fmt.Printf("\t%g\t%g\t%x\n", w, total, p)
	}
	w := model.Final(p)
	total += w
	// fmt.Printf("</s>\t%g\t%g\n", w, total)
	return
}
