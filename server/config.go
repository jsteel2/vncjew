package main

import "time"

var CFGAdminAccount = map[string]string{
	"admin": "***REMOVED***",
}
var CFGClientAccount = map[string]string{
	"client": "***REMOVED***",
	"admin": CFGAdminAccount["admin"],
}
var CFGMaxVNCConns = 100
var CFGMaxConcurrentOCR = 1
var CFGDb = "database.sqlite3"
var CFGClientPing = 5 * time.Second
var CFGClientTimeout = CFGClientPing * 2
var CFGIPInfoToken = "fd737c5e5030e3"
var CFGPasswords = []string{"123456", "password", "admin", "user", "default"}
var CFGVNCTimeout = "15"
var CFGVNCScreenshotBin = "./vncscreenshot"
var CFGTesseractBin = "tesseract"
