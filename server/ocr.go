package main

import (
	"os/exec"
)

var oLimit = make(chan struct{}, CFGMaxConcurrentOCR)
func oAcquire() { oLimit <- struct{}{} }
func oRelease() { <-oLimit }

func OCR(file string) string {
	oAcquire()
	defer oRelease()
	text, _ := exec.Command(CFGTesseractBin, file, "stdout").Output()
	return string(text)
}
