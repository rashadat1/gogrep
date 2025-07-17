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
		if pattern[0] == '(' {
			fmt.Printf("Calling extractGroupAndRest on: %q\n", pattern)
			group, after, ok := extractGroupAndRest(pattern)
			if !ok {
				return false
			}
			fmt.Printf("Extracting group (found '()'): %s, %s\n", group, after)
			if len(after) > 0 {
				switch after[0] {
				case '*':
					return matchStarGroup(group, after[1:], text)
				case '+':
					return matchPlusGroup(group, after[1:], text)
				case '?':
					return matchQuestionGroup(group, after[1:], text)
				}
			}
			matched, consumed := matchGroupOnce(group, text)
			if matched {
				return matchLoc(after, text[consumed:])
			}
			return false
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
func matchLocWithConsumption(pattern string, text []byte) (bool, int) {
	if pattern == "" {
		return true, 0
	}
	if pattern == "$" {
		return len(text) == 0, 0
	}
	if len(pattern) >= 2 {
		if pattern[1] == '*' {
			return matchStarWithConsumption(pattern[0], pattern[2:], text)
		}
		if pattern[1] == '+' {
			return matchPlusWithConsumption(pattern[0], pattern[2:], text)
		}
		if pattern[1] == '?' {
			return matchQuestionMarkWithConsumption(pattern[0], pattern[2:], text)
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
			switch after[0] {
			case '*':
				return matchStarGroupWithConsumption(group, after[1:], text)
			case '+':
				return matchPlusGroupWithConsumption(group, after[1:], text)
			case '?':
				return matchQuestionGroupWithConsumption(group, after[1:], text)
			}
			matched, consumed := matchGroupOnce(group, text)
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
func matchPlusCharGroupWithConsumption(group string, pattern string, text []byte) (bool, int) {
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
	}
	return false, 0
}
func matchNegativeCharacterGroupWithConsumption(pattern string, text []byte) (bool, int) {

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
func matchGroup(group, after string, text []byte) bool {
	fmt.Printf("matchGroup: group='%s' after='%s'\n", group, after)
	if strings.Contains(group, "|") {
		alternatives := splitAlternatives(group)
		fmt.Println(" -> alternatives:", alternatives)
		for _, alt := range alternatives {
			if matchLoc(alt+after, text) {
				return true
			}
		}
		return false
	}
	return matchLoc(group+after, text)
}

func matchGroupOnce(group string, text []byte) (bool, int) {
	alternatives := splitAlternatives(group)
	for _, alt := range alternatives {
		for i := 0; i <= len(text); i++ {
			fmt.Printf("Calling matchLoc from matchGroupOnce")
			if matchLoc(alt, text[:i]) {
				return true, i
			}
		}
	}
	return false, 0
}
func matchStarGroup(group, after string, text []byte) bool {
	i := 0
	for {
		if matchLoc(after, text[i:]) {
			return true
		}
		matched, consumed := matchGroupOnce(group, text[i:])
		if !matched || i+consumed > len(text) {
			break
		}
		i += consumed
	}
	return false
}
func matchPlusGroup(group, after string, text []byte) bool {
	matched, consumed := matchGroupOnce(group, text)
	if !matched {
		return false
	}
	i := consumed
	for {
		if matchLoc(after, text[i:]) {
			return true
		}
		matched, consumed = matchGroupOnce(group, text[i:])
		if !matched || i+consumed > len(text) {
			break
		}
		i += consumed
	}
	return false
}
func matchQuestionGroup(group, after string, text []byte) bool {
	if matchLoc(group, text) {
		return true
	}
	matched, consumed := matchGroupOnce(group, text)
	if matched && matchLoc(after, text[consumed:]) {
		return true
	}
	return false
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
func evalAltNested(pattern string) ([]string, error) {
	var parts [][]string
	i := 0
	for i < len(pattern) {
		switch {
		case pattern[i] == '(':
			j, err := findMatchingParenth(pattern, i)
			if err != nil {
				return nil, err
			}
			groupContent := pattern[i+1 : j]
			groupOpts := []string{}
			alts, err := topLevelAlternationSplit(groupContent)
			if err != nil {
				return nil, err
			}
			for _, alt := range alts {
				opt, err := evalAltNested(alt)
				if err != nil {
					return nil, err
				}
				groupOpts = append(groupOpts, opt...)
			}
			parts = append(parts, groupOpts)
			i = j + 1
		case pattern[i] == '|':
			return nil, errors.New("unexpected | outside parenthesis")
		default:
			start := i
			for i < len(pattern) && pattern[i] != '(' && pattern[i] != ')' {
				i++
			}
			literal := pattern[start:i]
			parts = append(parts, []string{literal})
		}
	}
	return cartesianProduct(parts), nil
}
func findMatchingParenth(s string, start int) (int, error) {
	level := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '(':
			level++
		case ')':
			level--
			if level == 0 {
				return i, nil
			}
		}
	}
	return -1, errors.New("unmatched '('")
}
func cartesianProduct(altGroups [][]string) []string {
	if len(altGroups) == 0 {
		return []string{}
	}
	if len(altGroups) == 1 {
		// only one alternating group
		return altGroups[0]
	}
	prev := altGroups[0]
	for i := 1; i < len(altGroups); i++ {
		if len(altGroups[i]) != 0 {
			prev = productCalc(prev, altGroups[i])

		}
	}
	return prev
}
func productCalc(x, y []string) []string {
	res := make([]string, 0)
	for _, elx := range x {
		for _, ely := range y {
			res = append(res, elx+ely)
		}
	}
	return res
}
