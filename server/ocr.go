package main

import (
	"context"
	"log"
	"os"
	"os/exec"

	vision "cloud.google.com/go/vision/apiv1"
)

var oLimit = make(chan struct{}, CFGMaxConcurrentOCR)
func oAcquire() { oLimit <- struct{}{} }
func oRelease() { <-oLimit }

func googleOCR(file string) (string, error) {
	ctx := context.Background()

	client, err := vision.NewImageAnnotatorClient(ctx)
	if err != nil {
		return "", err
	}

	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	image, err := vision.NewImageFromReader(f)
	if err != nil {
		return "", err
	}

	annotations, err := client.DetectTexts(ctx, image, nil, 10)
	if err != nil {
		return "", err
	}

	return annotations[0].Description, nil
}

func tesseractOCR(file string) string {
	text, _ := exec.Command(CFGTesseractBin, file, "stdout").Output()
	return string(text)
}

func OCR(file string) string {
	oAcquire()
	defer oRelease()
	res, err := googleOCR(file)
	if err != nil {
		log.Println(err)
		return tesseractOCR(file)
	}
	return res
}
