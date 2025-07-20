package main

import (
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
	fmt.Printf("Captured group that is referred to: %s\n", captured)
	if !ok {
		return false, 0
	}
	if len(text) < len(captured) {
		return false, 0
	}
	if string(text[:len(captured)]) == captured {
		fmt.Printf("We have this is the subset of the text: %s\n", text[:len(captured)])
		matched, consumed := matchLocWithConsumption(afterBackRef, text[len(captured):])
		fmt.Printf("After backreference: %s\n", afterBackRef)
		fmt.Printf("Pattern after backreference: %s\n", string(text[len(captured):]))
		if matched {
			return true, len(captured) + consumed
		}
	}
	return false, 0
}
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
	altParts, err := topLevelAlternationSplit(pattern)
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(2)
	}
	for _, sub := range altParts {
		ok := matchingEngine(toSearchBytes, sub)
		if ok {
			os.Exit(0)
		}
	}
	os.Exit(1)
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
			switch after[0] {
			case '*':
				return matchStarGroupWithConsumption(group, after[1:], text, currGroupNum)
			case '+':
				return matchPlusGroupWithConsumption(group, after[1:], text, currGroupNum)
			case '?':
				return matchQuestionGroupWithConsumption(group, after[1:], text, currGroupNum)
			}
			matched, consumed := matchGroupOnce(group, text, currGroupNum)
			if matched {
				afterMatched, afterConsumed := matchLocWithConsumption(after, text[consumed:])
				if afterMatched {
					return true, afterConsumed + consumed
				}
			}
			return false, 0
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
	i := 0
	for {
		if i == 0 {
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
		if i > 0 {
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
	// match 0 or more occurrences of byte c -> at each step check if we match the rest of the pattern
	// break if we reach i past the length of text without matching the remaining text or if c does not match
	// the text anymore
	i := 0
	for {
		matched, consumed := matchLocWithConsumption(patternAfterStar, text[i:])
		if matched {
			return true, i + consumed
		}
		if len(text) <= i || (c != '.' && c != text[i]) {
			break
		}
		i++
	}
	return false, 0
}
func matchPlusWithConsumption(c byte, patternAfterPlus string, text []byte) (bool, int) {
	// match one or more occurrence
	i := 0
	for {
		if i == 0 && (len(text) < 1 || (c != '.' && c != text[i])) {
			// check if matches 1 occurrence at beginning of text
			break
		}
		if i > 0 {
			matched, numConsumed := matchLocWithConsumption(patternAfterPlus, text[i:])
			if matched {
				return true, numConsumed + i
			}

			if len(text) <= i || (c != '.' && c != text[i]) {
				break
			}
			i++

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
	for {
		if i == 0 && !strings.Contains(group, string(text[0])) {
			break
		}
		if i > 0 {
			matched, consumed := matchLocWithConsumption(pattern, text[i:])
			if matched {
				return true, consumed + i
			}
			if len(text) <= i || !strings.Contains(group, string(text[0])) {
				break
			}
		}
		i++
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
	for j := i; j >= 0; j-- {
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
func matchGroupOnce(group string, text []byte, groupNum int) (bool, int) {
	alternatives := splitAlternatives(group)
	for _, alt := range alternatives {
		// just try to match the alternative by starting at the current position
		matched, consumed := matchLocWithConsumption(alt, text)
		if matched {
			globCaptureContext.captures[groupNum] = string(text[:consumed])
			return true, consumed
		}
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
