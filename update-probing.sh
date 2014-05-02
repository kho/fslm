#!/bin/sh -e

gofmt -w -r '__Key -> WordId' probing_impl.go
gofmt -w -r '__Value -> StateWeight' probing_impl.go
gofmt -w -r '__KEY_NIL -> WORD_NIL' probing_impl.go
gofmt -w -r '__Hash -> WordIdHash' probing_impl.go
gofmt -w -r '__Equal -> WordIdEqual' probing_impl.go
sed -i '' -e 's/package probing/package fslm/' probing_impl.go
sed -i '' -e 's/__/xqw/g' probing_impl.go
gofmt -w -r 'NewxqwMap -> newXqwMap' probing_impl.go
