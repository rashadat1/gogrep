package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)


func main() {
	// usage echo <input_string> | ./gogrep -E <pattern>
	if len(os.Args) < 3 || os.Args[1] != "-E" {
		// exit with code 2 on error 
		os.Exit(2)
	}
	pattern := os.Args[2]
	toSearchBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input text: %v\n", err)
		os.Exit(2)
	}
	ok, err := matchLine(toSearchBytes, pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if !ok {
		// no match
		os.Exit(1)
	}
}
func matchLine(line []byte, pattern string) (bool, error) {
	var ok bool
	switch {
	case pattern == "\\d":
		ok = checkDigit(line)
	case pattern == "\\w":
		ok = checkAlphaNumeric(line)
	case strings.HasPrefix(pattern, "[^"):
		pattern = strings.Trim(strings.Trim(pattern, "[]"), "^")
		ok = checkCompletementMatchSet(line, pattern)
	default:
		ok = bytes.ContainsAny(line, pattern)
	}
	return ok, nil

}
func checkDigit(line []byte) bool {
	for _, l := range line {
		if unicode.IsDigit(rune(l)) {
			return true
		}
	}
	return false
}
func checkAlphaNumeric(line []byte) bool {
	for _, l := range line {
		if unicode.IsDigit(rune(l)) || unicode.IsLetter(rune(l)) || string(l) == "_" {
			return true
		}
	}
	return false
}
func checkCompletementMatchSet(line []byte, matchSet string) bool {
	for _, l := range line {
		if !strings.Contains(matchSet, string(l)) {
			return true
		}
	}
	return false
}
