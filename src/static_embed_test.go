package main

import (
	"bytes"
	"testing"
)

func TestEmbeddedFrontendContainsIndex(t *testing.T) {
	content, err := frontendFiles.ReadFile("webdist/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(bytes.ToLower(content), []byte("<!doctype html>")) {
		t.Fatal("embedded H5 index is not a valid build artifact")
	}
}
