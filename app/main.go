package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var globCaptureContext *CaptureContext

type CaptureContext struct {
	captures map[int]string
	groupNum int
}

func newCaptureContext() *CaptureContext {
	return &CaptureContext{
		captures: make(map[int]string),
		groupNum: 0,
	}
}
func matchBackReference(groupNumSearch int, text []byte, afterBackRef string) (bool, int) {
	captured, ok := globCaptureContext.captures[groupNumSearch]
	if !ok {
		return false, 0
	}
	if len(text) < len(captured) {
		return false, 0
	}
	if string(text[:len(captured)]) == captured {
		matched, consumed := matchLocWithConsumption(afterBackRef, text[len(captured):])
		if matched {
			return true, len(captured) + consumed
		}
	}
	return false, 0
}
func main() {
	// usage echo <input_string> | ./gogrep -E <pattern>
	var pattern string
	useFile := false
	files := make(map[string]*os.File, 0)
	recursive := false
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Error: Not enough arguments")
		os.Exit(2)
	}
	if os.Args[1] == "-r" {
		recursive = true
	}
	if !recursive && os.Args[1] != "-E" {
		fmt.Fprintf(os.Stderr, "Error: Missing required parameter -E")
		os.Exit(2)
	}
	if recursive {
		if os.Args[2] != "-E" {
			fmt.Fprintf(os.Stderr, "Error: Missing required parameter -E")
			os.Exit(2)
		}
	}
	start := 0
	if recursive {
		pattern = os.Args[3]
		start = 4
	} else {
		pattern = os.Args[2]
		start = 3
	}
	if len(os.Args) >= 4 {
		for j := start; j < len(os.Args); j++ {
			fileName := os.Args[j]
			fileInfo, err := os.Stat(fileName)
			if os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "File at file_path <%s> does not exist\n", fileName)
				os.Exit(2)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error processing file_path <%s>: %s\n", fileName, err)
				os.Exit(2)
			}
			if !fileInfo.IsDir() {
				fileObj, err := os.Open(fileName)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error opening file at file_path <%s>: %s\n", fileName, err)
					os.Exit(2)
				}
				files[fileName] = fileObj
			} else {
				// file path is a directory - in which case we need to recursively add the files in the subdirectories
				dirPath := fileName
				files, err = getFilesRecursively(dirPath, files)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error retrieving files in recursive call: %s\n", err)
					os.Exit(2)
				}
			}

		}
		useFile = true
	}
	if !useFile {
		files["os.stdin"] = os.Stdin
	}
	linesMatchingPattern := make([]string, 0)

	for fileName, fileObj := range files {
		toSearchBytes, err := io.ReadAll(fileObj)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input text: %v\n", err)
			os.Exit(2)
		}
		separatedLines := bytes.Split(toSearchBytes, []byte("\n"))
		altParts, err := topLevelAlternationSplit(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
		for _, sub := range altParts {
			for _, line := range separatedLines {
				prefixedLine := string(line)
				if len(files) > 1 {
					prefixedLine = fileName + ":" + prefixedLine
				}
				ok := matchingEngine(line, sub)
				if ok {
					linesMatchingPattern = append(linesMatchingPattern, string(prefixedLine))
				}
			}
		}

	}
	if len(linesMatchingPattern) == 0 {
		os.Exit(1)
	}
	for i := 0; i < len(linesMatchingPattern); i++ {
		fmt.Println(linesMatchingPattern[i])
	}
}
func getFilesRecursively(dirPath string, allFiles map[string]*os.File) (map[string]*os.File, error) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading directory <%s> contents: %s", dirPath, err)
	}
	if !strings.HasSuffix(dirPath, "/") {
		dirPath = dirPath + "/"
	}
	for _, file := range files {
		dirElementPath := dirPath + file.Name()
		if file.IsDir() {
			allFiles, err = getFilesRecursively(dirElementPath, allFiles)
		} else {
			fileObj, err := os.Open(dirElementPath)
			if err != nil {
				return nil, err
			}
			allFiles[dirElementPath] = fileObj
		}
	}
	return allFiles, nil

}
func matchingEngine(text []byte, pattern string) bool {
	globCaptureContext = newCaptureContext()
	if len(pattern) > 1 && pattern[0] == '^' {
		matched, _ := matchLocWithConsumption(pattern[1:], text)
		return matched
	}
	for i := 0; i < len(text); i++ {
		matched, _ := matchLocWithConsumption(pattern, text[i:])
		if matched {
			return true
		}
	}
	return false
}
func matchLocWithConsumption(pattern string, text []byte) (bool, int) {
	if pattern == "" {
		return true, 0
	}
	if pattern == "$" {
		return len(text) == 0, 0
	}
	if len(pattern) >= 2 {
		if pattern[0] == '\\' && len(pattern) >= 3 {
			// handle escape sequences with quantifiers
			if pattern[2] == '*' {
				return matchStarCharClassWithConsumption(pattern, pattern[3:], text)
			}
			if pattern[2] == '+' {
				return matchPlusCharClassWithConsumption(pattern, pattern[3:], text)
			}
			if pattern[2] == '?' {
				return matchQuestionCharClassWithConsumption(pattern, pattern[3:], text)
			}
		}
		// handle regular characters with quantifiers
		if pattern[1] == '*' {
			return matchStarWithConsumption(pattern[0], pattern[2:], text)
		}
		if pattern[1] == '+' {
			return matchPlusWithConsumption(pattern[0], pattern[2:], text)
		}
		if pattern[1] == '?' {
			return matchQuestionMarkWithConsumption(pattern[0], pattern[2:], text)
		}
		if pattern[0] == '\\' {
			// try to detect and match backreference
			num, err := strconv.Atoi(pattern[1:2])
			if err == nil {
				return matchBackReference(num, text, pattern[2:])
			}
		}
		if pattern[0] == '[' {
			if pattern[1] == '^' {
				return matchNegativeCharacterGroupWithConsumption(pattern[2:], text)
			}
			return matchCharacterGroupWithConsumption(pattern[1:], text)
		}
		if pattern[0] == '(' {
			group, after, ok := extractGroupAndRest(pattern)
			if !ok {
				return false, 0
			}
			globCaptureContext.groupNum++
			currGroupNum := globCaptureContext.groupNum
			if len(after) >= 1 {
				switch after[0] {
				case '*':
					return matchStarGroupWithConsumption(group, after[1:], text, currGroupNum)
				case '+':
					return matchPlusGroupWithConsumption(group, after[1:], text, currGroupNum)
				case '?':
					return matchQuestionGroupWithConsumption(group, after[1:], text, currGroupNum)
				}
			}
			return matchGroupWithBacktracking(group, after, text, currGroupNum)
		}
	}
	if len(text) > 0 {
		if pattern[0] == '.' {
			matched, consumed := matchLocWithConsumption(pattern[1:], text[1:])
			if matched {
				return true, consumed + 1
			}
		}
		r, size := utf8.DecodeRuneInString(string(text))
		pass, numConsumed := matchCharWithRune(pattern, r)
		if pass {
			matched, consumed := matchLocWithConsumption(pattern[numConsumed:], text[size:])
			if matched {
				return true, size + consumed
			}
		}
		return false, 0
	}
	return false, 0
}
func matchStarCharClassWithConsumption(wholePattern, patternAfter string, text []byte) (bool, int) {
	i := 0
	for {
		if i >= len(text) {
			return false, 0
		}
		matched, consumed := matchLocWithConsumption(patternAfter, text[i:])
		if matched {
			return true, consumed + i
		}
		switch {
		case strings.HasPrefix(wholePattern, "\\d"):
			r, size := utf8.DecodeRuneInString(string(text[i:]))
			if !unicode.IsDigit(r) {
				return false, 0
			}
			i += size
		case strings.HasPrefix(wholePattern, "\\w"):
			r, size := utf8.DecodeRuneInString(string(text[i:]))
			if !(r >= 'A' && r <= 'Z') &&
				!(r >= 'a' && r <= 'z') &&
				!(r >= '0' && r <= '9') &&
				r != '_' {
				return false, 0
			}
			i += size
		default:
			return false, 0
		}
	}
}
func matchPlusCharClassWithConsumption(wholePattern, patternAfter string, text []byte) (bool, int) {
	matchHelper := func(data []byte, offset int) (bool, int) {
		if offset >= len(data) {
			return false, 0
		}
		r, size := utf8.DecodeRuneInString(string(data[offset:]))
		switch {
		case strings.HasPrefix(wholePattern, "\\d"):
			return unicode.IsDigit(r), size
		case strings.HasPrefix(wholePattern, "\\w"):
			return (r >= 'A' && r <= 'Z') ||
				(r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') ||
				r == '_', size
		default:
			return false, 0
		}
	}
	i := 0
	// check if we match at least one character required for '+'
	matches, size := matchHelper(text, 0)
	if !matches {
		return false, 0
	}
	i += size
	// now consume as many characters as possible greedily
	for {
		matches, size := matchHelper(text, i)
		if !matches {
			break
		}
		i += size
	}
	// now we try to match from the point where we exit the loop - backtracking if need be
	for j := i; j >= 1; {
		matched, consumed := matchLocWithConsumption(patternAfter, text[j:])
		if matched {
			return true, consumed + j
		}
		if j == 1 {
			// need to match at least one character
			break
		}
		j--
		for j > 0 && !utf8.RuneStart(text[j]) {
			// while we are greater than 0 and we are not at a byte that could start a
			// unicode character
			j--
		}
	}
	return false, 0
}
func matchQuestionCharClassWithConsumption(wholePattern, patternAfter string, text []byte) (bool, int) {
	switch {
	case strings.HasPrefix(wholePattern, "\\d"):
		r, size := utf8.DecodeRuneInString(string(text))
		if unicode.IsDigit(r) {
			if matched, consumed := matchLocWithConsumption(patternAfter, text[size:]); matched {
				return true, consumed + size
			}
		}
		return matchLocWithConsumption(patternAfter, text)
	case strings.HasPrefix(wholePattern, "\\w"):
		r, size := utf8.DecodeRuneInString(string(text))
		if (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '_' {
			if matched, consumed := matchLocWithConsumption(patternAfter, text[size:]); matched {
				return true, consumed + size
			}
		}
		return matchLocWithConsumption(patternAfter, text)

	default:
		return false, 0
	}
}
func matchStarGroupWithConsumption(group, after string, text []byte, groupNum int) (bool, int) {
	i := 0
	for {
		matched, consumed := matchLocWithConsumption(after, text[i:])
		if matched {
			return true, consumed + i
		}
		groupMatched, groupConsumed := matchGroupOnce(group, text[i:], groupNum)
		if !groupMatched || i+groupConsumed > len(text) {
			break
		}
		i = i + groupConsumed
	}
	return false, 0
}
func matchPlusGroupWithConsumption(group, after string, text []byte, groupNum int) (bool, int) {
	matched, consumed := matchGroupOnce(group, text, groupNum)
	if !matched {
		return false, 0
	}
	i := consumed
	for {
		matched, cons := matchLocWithConsumption(after, text[i:])
		if matched {
			return true, i + cons
		}
		groupMatched, groupConsumed := matchGroupOnce(group, text[i:], groupNum)
		if !groupMatched || i+groupConsumed > len(text) {
			break
		}
		i = i + groupConsumed
	}
	return false, 0
}
func matchQuestionGroupWithConsumption(group, after string, text []byte, groupNum int) (bool, int) {
	groupMatched, groupConsumed := matchGroupOnce(group, text, groupNum)
	if groupMatched {
		matched, consumed := matchLocWithConsumption(after, text[groupConsumed:])
		if matched {
			return true, groupConsumed + consumed
		}
	}
	matched, consumed := matchLocWithConsumption(after, text)
	if matched {
		return true, consumed
	}
	return false, 0
}
func matchStarWithConsumption(c byte, patternAfterStar string, text []byte) (bool, int) {
	// match 0 or more occurrences of byte c
	i := 0
	// consume as many characters as we can greedily
	for i < len(text) && (c == '.' || c == text[i]) {
		i++
	}
	// once we finish - begin the backtrack
	for j := i; j >= 0; j-- {
		matched, consumed := matchLocWithConsumption(patternAfterStar, text[j:])
		if matched {
			return true, j + consumed
		}
	}
	return false, 0
}
func matchPlusWithConsumption(c byte, patternAfterPlus string, text []byte) (bool, int) {
	// match one or more occurrence
	i := 0
	if len(text) < 1 || (c != '.' && c != text[i]) {
		return false, 0
	}
	i++
	for i < len(text) && (c == '.' || c == text[i]) {
		i++
	}
	for j := i; j >= 1; j-- {
		matched, consumed := matchLocWithConsumption(patternAfterPlus, text[j:])
		if matched {
			return true, consumed + j
		}
	}
	return false, 0
}
func matchQuestionMarkWithConsumption(c byte, patternAfterQuestion string, text []byte) (bool, int) {
	// try to match 1 occurrence first
	if len(text) > 0 && (c == text[0] || c == '.') {
		matched, numConsumed := matchLocWithConsumption(patternAfterQuestion, text[1:])
		if matched {
			return true, numConsumed + 1
		}
	}
	// fallback to matching 0 occurrences
	matched, numConsumed := matchLocWithConsumption(patternAfterQuestion, text)
	if matched {
		return true, numConsumed
	}
	return false, 0
}
func matchCharacterGroupWithConsumption(pattern string, text []byte) (bool, int) {
	if len(text) == 0 {
		return false, 0
	}
	if len(pattern) == 0 {
		if text[0] == '[' {
			return true, 1
		}
		return false, 0
	}
	i := 0
	charactersInGroup := ""
	for {
		if i >= len(pattern) {
			break
		}
		if pattern[i] == ']' {
			if i+1 < len(pattern) {
				if pattern[i+1] == '*' {
					return matchStarCharGroupWithConsumption(charactersInGroup, pattern[i+2:], text)
				}
				if pattern[i+1] == '+' {
					return matchPlusCharGroupWithConsumption(charactersInGroup, pattern[i+2:], text)
				}
			}
			if strings.Contains(charactersInGroup, string(text[0])) {
				matched, consumed := matchLocWithConsumption(pattern[i+1:], text[1:])
				if matched {
					return true, consumed + 1
				}
			}
			return false, 0
		}
		charactersInGroup = charactersInGroup + string(pattern[i])
		i++
	}
	return false, 0
}
func matchStarCharGroupWithConsumption(group string, pattern string, text []byte) (bool, int) {
	i := 0
	for {
		matched, consumed := matchLocWithConsumption(pattern, text[i:])
		if matched {
			return true, consumed + i
		}
		if len(text) <= i || !strings.Contains(group, string(text[i])) {
			break
		}
		i++
	}
	return false, 0
}
func matchPlusCharGroupWithConsumption(group, pattern string, text []byte) (bool, int) {
	i := 0
	// First, ensure we match at least one character
	if len(text) == 0 || !strings.Contains(group, string(text[0])) {
		return false, 0
	}
	i = 1

	// Then consume as many characters as possible
	for i < len(text) && strings.Contains(group, string(text[i])) {
		i++
	}

	// Now try to match the rest of the pattern, backtracking if necessary
	for j := i; j >= 1; j-- {
		matched, consumed := matchLocWithConsumption(pattern, text[j:])
		if matched {
			return true, consumed + j
		}
	}
	return false, 0
}
func matchNegativeCharacterGroupWithConsumption(pattern string, text []byte) (bool, int) {
	if len(text) == 0 {
		return false, 0
	}
	if len(pattern) == 0 {
		if len(text) == 2 {
			return text[0] == '[' && text[1] == '^', 2
		}
		return false, 0
	}
	negCharacterGroup := ""
	i := 0
	for {
		if len(pattern) <= i {
			break
		}
		if pattern[i] == ']' {
			if len(pattern) > i+1 {
				if pattern[i+1] == '*' {
					return matchStarNegCharGroupWithConsumption(negCharacterGroup, pattern[i+2:], text)
				}
				if pattern[i+1] == '+' {
					return matchPlusNegCharGroupWithConsumption(negCharacterGroup, pattern[i+2:], text)
				}
			}
			if !strings.Contains(negCharacterGroup, string(text[0])) {
				matched, consumed := matchLocWithConsumption(pattern[i+1:], text[1:])
				if matched {
					return true, consumed + 1
				}
				return false, 0
			}
		}
		negCharacterGroup = negCharacterGroup + string(pattern[i])
		i++
	}
	return false, 0
}
func matchStarNegCharGroupWithConsumption(negGroup, patternAfter string, text []byte) (bool, int) {
	i := 0
	for i < len(text) && !strings.Contains(negGroup, string(text[i])) {
		i++
	}
	for j := i; j >= 0; j-- {
		matched, consumed := matchLocWithConsumption(patternAfter, text[j:])
		if matched {
			return true, consumed + j
		}
	}
	return false, 0
}
func matchPlusNegCharGroupWithConsumption(negGroup, patternAfter string, text []byte) (bool, int) {
	if len(text) == 0 || strings.Contains(negGroup, string(text[0])) {
		return false, 0
	}
	i := 1
	for i < len(text) && !strings.Contains(negGroup, string(text[i])) {
		i++
	}
	if patternAfter == "" {
		return true, i
	}
	for j := i; j >= 1; j-- {
		matched, consumed := matchLocWithConsumption(patternAfter, text[j:])
		if matched {
			return true, consumed + j
		}
	}
	return false, 0
}
func splitAlternatives(group string) []string {
	var result []string
	start := 0
	depth := 0
	for i := 0; i < len(group); i++ {
		switch {
		case group[i] == '(':
			depth++
		case group[i] == ')':
			depth--
		case group[i] == '|':
			if depth == 0 {
				result = append(result, group[start:i])
				start = i + 1
			}
		}
	}
	result = append(result, group[start:])
	return result
}
func matchGroupWithBacktracking(group, after string, text []byte, groupNum int) (bool, int) {
	// special handling for negative char groups with + quantifier
	if strings.HasPrefix(group, "[^") && strings.HasSuffix(group, "+") {
		// extract the negative character set
		closeBracket := strings.Index(group[2:], "]")
		if closeBracket == -1 {
			return false, 0
		}
		negCharSet := group[2 : closeBracket+2]
		// find max consumption
		if len(text) == 0 || strings.Contains(negCharSet, string(text[0])) {
			return false, 0
		}
		maxConsume := 1
		for maxConsume < len(text) && !strings.Contains(negCharSet, string(text[maxConsume])) {
			maxConsume++
		}

		for consume := maxConsume; consume >= 1; consume-- {
			globCaptureContext.captures[groupNum] = string(text[:consume])
			afterMatched, afterConsumed := matchLocWithConsumption(after, text[consume:])
			if afterMatched {
				return true, afterConsumed + consume
			}
		}
		return false, 0

	}
	// regular group matching
	matched, consumed := matchGroupOnce(group, text, groupNum)
	if matched {
		afterMatched, afterConsumed := matchLocWithConsumption(after, text[consumed:])
		if afterMatched {
			return true, consumed + afterConsumed
		}
	}
	return false, 0
}
func matchGroupOnce(group string, text []byte, groupNum int) (bool, int) {
	alternatives := splitAlternatives(group)
	for _, alt := range alternatives {
		// save the current group number context before recursive matching
		savedGroupNum := globCaptureContext.groupNum
		matched, consumed := matchLocWithConsumption(alt, text)
		if matched {
			globCaptureContext.captures[groupNum] = string(text[:consumed])
			return true, consumed
		}
		// restore the group number context if matching failed
		globCaptureContext.groupNum = savedGroupNum
	}
	return false, 0
}
func extractGroupAndRest(pattern string) (string, string, bool) {
	if len(pattern) == 0 || pattern[0] != '(' {
		return "", "", false
	}
	depth := 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return pattern[1:i], pattern[i+1:], true
			}
		}
	}
	return "", "", false
}
func matchCharWithRune(pattern string, r rune) (bool, int) {
	switch {
	case strings.HasPrefix(pattern, "\\d"):
		return unicode.IsDigit(r), 2
	case strings.HasPrefix(pattern, "\\w"):
		return (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			(r == '_'), 2
	default:
		return rune(pattern[0]) == r, 1
	}
}
func topLevelAlternationSplit(pattern string) ([]string, error) {
	parenthLevel := 0
	parts := make([]string, 0)
	for i, char := range pattern {
		switch char {
		case '(':
			parenthLevel++
		case ')':
			if parenthLevel == 0 {
				return nil, errors.New("unmatched parenthesis braces")
			}
			parenthLevel--
		case '|':
			if parenthLevel == 0 {
				l := pattern[:i]
				r := pattern[i+utf8.RuneLen(char):]
				leftParts, err1 := topLevelAlternationSplit(l)
				rightParts, err2 := topLevelAlternationSplit(r)
				if err1 != nil || err2 != nil {
					return nil, errors.New(err1.Error() + err2.Error())
				}
				parts = append(parts, leftParts...)
				parts = append(parts, rightParts...)
				return parts, nil
			}
		}
	}
	return []string{pattern}, nil
}
