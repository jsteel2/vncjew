package main

import (
	"os/exec"
)

var limit = make(chan struct{}, CFGMaxConcurrentOCR)
func acquire() { limit <- struct{}{} }
func release() { <-limit }

func OCR(file string) string {
	acquire()
	defer release()
	text, _ := exec.Command(CFGTesseractBin, file, "stdout").Output()
	return string(text)
}
