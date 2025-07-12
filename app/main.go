package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
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
	ok := matchingEngine(toSearchBytes, pattern)
	if !ok {
		// no match
		os.Exit(1)
	}
}
func matchingEngine(text []byte, pattern string) bool {
	if len(pattern) > 1 && pattern[0] == '^' {
		// match at beginning
		if matchLoc(pattern[1:], text) {
			return true
		}

	}
	for i := 0; i <= len(text); i++ {
		// iteratively attempt to match at each location
		if matchLoc(pattern, text[i:]) {
			return true
		}
	}
	return false
}
func matchLoc(pattern string, text []byte) bool {
	if pattern == "" {
		// if we have exhausted the pattern without having some exception
		return true
	}
	if pattern == "$" {
		// match the end if we are at the end of the text
		return len(text) == 0
	}
	if len(pattern) >= 2 {
		if pattern[1] == '*' {
			return matchStar(pattern[0], pattern[2:], text)
		}
		if pattern[1] == '+' {
			return matchPlus(pattern[0], pattern[2:], text)
		}
		if pattern[1] == '?' {
			return matchQuestionMark(pattern[0], pattern[2:], text)
		}
		if pattern[0] == '[' {
			if pattern[1] == '^' {
				return matchNegativeCharacterGroup(pattern[2:], text)
			}
			return matchCharacterGroup(pattern[1:], text)
		}
	}
	if len(text) > 0 {
		if pattern[0] == '.' {
			return matchLoc(pattern[1:], text[1:])
		}
		// because some characters are more than one byte we cannot
		// naively consume and compare one byte from text we consume
		// runes at a time
		r, size := utf8.DecodeRuneInString(string(text))
		pass, numConsumed := matchCharWithRune(pattern, r)
		return pass && matchLoc(pattern[numConsumed:], text[size:])
	}
	return false

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
func matchChar(pattern string, text []byte) (bool, int) {
	if len(text) == 0 {
		return false, 0
	}
	switch {
	case strings.HasPrefix(pattern, "\\d"):
		return unicode.IsDigit(rune(text[0])), 2
	case strings.HasPrefix(pattern, "\\w"):
		return unicode.IsDigit(rune(text[0])) || unicode.IsLetter(rune(text[0])) || string(text[0]) == "_", 2
	default:
		return pattern[0] == text[0], 1
	}
}
func matchPlus(c byte, patternAfterPlus string, text []byte) bool {
	i := 0
	for {
		if i == 0 && c != '.' && c != text[i] {
			// if we don't match at least one occurrence
			break
		}
		if i > 0 && matchLoc(patternAfterPlus, text[i:]) {
			return true
		}
		if i >= len(text) || (c != '.' && c != text[i]) {
			// reach end of text or the pattern after does not match and we have conflict
			break
		}
		i++
	}
	return false
}
func matchStar(c byte, patternAfterStar string, text []byte) bool {
	i := 0
	for {
		if matchLoc(patternAfterStar, text[i:]) {
			return true
		}
		if i >= len(text) || (c != '.' && c != text[i]) {
			break
		}
		i++
	}
	return false
}
func matchCharacterGroup(pattern string, text []byte) bool {
	if len(text) == 0 {
		return false
	}
	if len(pattern) == 0 {
		return text[0] == '['
	}
	i := 0
	characterGroup := ""
	for {
		if i >= len(pattern) {
			break
		}
		if pattern[i] == ']' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				return matchStarCharGroup(characterGroup, pattern[i+2:], text)
			}
			if strings.Contains(characterGroup, string(text[0])) {
				return matchLoc(pattern[i+1:], text[1:])
			}
		}
		characterGroup += string(pattern[i]) // build character group
		i++

	}
	return false
}
func matchQuestionMark(c byte, patternAfterQuestion string, text []byte) bool {
	// match 0 occurrences
	if matchLoc(patternAfterQuestion, text) {
		return true
	}
	if len(text) > 0 && (c == '.' || c == text[0]) {
		// match 1 occurrence
		if matchLoc(patternAfterQuestion, text[1:]) {
			return true
		}
	}
	return false
}
func matchNegativeCharacterGroup(pattern string, text []byte) bool {
	if len(text) == 0 {
		return false
	}
	if len(pattern) == 0 {
		if len(text) == 2 {
			return text[0] == '[' && text[1] == '^'
		}
	}
	i := 0
	negCharacterGroup := ""
	for {
		if i >= len(pattern) {
			break
		}
		if pattern[i] == ']' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				return matchStarNegCharGroup(negCharacterGroup, pattern[i+2:], text)
			}
			if !strings.Contains(negCharacterGroup, string(text[0])) {
				return matchLoc(pattern[i+1:], text[1:])
			}
		}
		negCharacterGroup += string(pattern[i])
		i++
	}
	return false
}

/*
	func matchStar(c byte, patternAfterStar string, text []byte) bool {
		i := 0
		for {
			if matchLoc(patternAfterStar, text[i:]) {
				return true
			}
			if i >= len(text) || (c != '.' && c != text[i]) {
				break
			}
			i++
		}
		return false
	}
*/
func matchStarCharGroup(characterGroup string, patternAfterStar string, text []byte) bool {
	i := 0
	for {
		if matchLoc(patternAfterStar, text[i:]) {
			return true
		}
		if i >= len(text) || !strings.Contains(characterGroup, string(text[i])) {
			break
		}
		i++
	}
	return false
}
func matchStarNegCharGroup(negCharacterGroup string, patternAfterStar string, text []byte) bool {
	i := 0
	for {
		if matchLoc(patternAfterStar, text[i:]) {
			return true
		}
		if i >= len(text) || strings.Contains(negCharacterGroup, string(text[i])) {
			break
		}
		i++
	}
	return false
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
				return nil, errors.New("Unmatched Parenthesis braces")
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
func evalAlt(pattern string) [][]string {
	// called exclusively on alt options returned by topLevelAlternationSplit that contain '|'
	// this will be parts with '|' inside ()
	prefix := "" // before alternation group
	resElement := ""
	inAlternationGroup := false
	group := make([]string, 0)
	res := make([][]string, 0)
	for _, char := range pattern {
		switch {
		case char == '(':
			inAlternationGroup = true
		case char == '|':
			group = append(group, prefix+resElement) // characters outside alternation group and the alternation group up to |
			resElement = ""                          // make empty because the characters between ( and | are already added now
		case char == ')':
			if inAlternationGroup {
				inAlternationGroup = false
			}
			if len(resElement) > 0 {
				group = append(group, prefix+resElement) // if we have accumulated any more elements since the last |
			}
			resElement = ""
			prefix = ""
			res = append(res, group)
			group = []string{} // start over because we are finished with this alternation group
		default:
			if !inAlternationGroup {
				prefix = prefix + string(char)
			} else {
				resElement = resElement + string(char)
			}
		}
	}
	if len(group) > 0 {
		// in this case there are no parentheses I think this is redundant as we are only parsing stuff that has '|' within () and we know the parens are valid
		// so this should never happen
		res = append(res, group)
	}
	if len(prefix) > 0 && len(res) > 0 {
		suffix := prefix
		// there is a non-alternating group suffix we need to add
		for i, altGroup := range res[len(res)-1] {
			res[len(res)-1][i] = altGroup + suffix
		}
	}
	return res
}
