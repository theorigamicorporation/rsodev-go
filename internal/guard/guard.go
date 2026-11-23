// Package guard keeps names out of this repository that do not belong in
// public code: the operator's own infrastructure and the products the
// platform is built on. The names are stored as hashes so that this file
// does not publish them either.
package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

// words are refused as whole words (a trailing plural "s" is ignored).
var words = map[string]struct{}{
	"08ee372c78b03947": {},
	"0d2e1833a2457c8b": {},
	"168a3276263242e0": {},
	"1b2501a20fe1bcd8": {},
	"2870234254a0a277": {},
	"28c5a51eb38765d9": {},
	"4637af0f498356f4": {},
	"4d09264a5878d697": {},
	"4ebde790877e2f92": {},
	"58936b7de82a968f": {},
	"5deb51107daf1235": {},
	"613469788684463f": {},
	"7386dca777a79e7e": {},
	"76e3c7bfe641ea12": {},
	"7c15cc86b846a0b8": {},
	"86bd33ebf64e01aa": {},
	"8873184952915b68": {},
	"938d4b249e2e19e7": {},
	"a14ee6ffaf37a3e2": {},
	"a2e10207c7be30e1": {},
	"c8cf701f30ad11d3": {},
	"ce32e15bb8bbb798": {},
	"ddc474fe1ec413bb": {},
	"de6f0783531f8bc8": {},
	"e1b43c3d6862996f": {},
	"e6670506c75ce37b": {},
	"e68354bbc323e050": {},
	"edd0dfd02b346db0": {},
	"f5cfcb570b7edac2": {},
	"fc40fd8b1bfdc14c": {},
}

// fragments are refused anywhere inside a word.
var fragments = map[string]struct{}{
	"07a07ff2a800af32": {},
	"19fdb2b925dd8599": {},
	"2ce1d14038a732cc": {},
	"4d434f6d36c01ca0": {},
	"5648b3be51b2a73b": {},
	"738e22f0acab814c": {},
	"7b173343f539cdc4": {},
	"858bd5f51000cf27": {},
	"92386da207cbacd2": {},
	"a5c1835f2f1f4d4d": {},
	"a8986170949175f0": {},
	"b55284c9f6683bd2": {},
	"be6d9270de402b9f": {},
	"d20dbd6a870c2519": {},
	"d7fa96b0923a796d": {},
	"e726c5854df85580": {},
}

// fragmentLengths are the lengths of the fragments, to slide a window by.
var fragmentLengths = []int{6, 7, 8, 9, 10, 11, 13}

var privateAddress = regexp.MustCompile(`\b(10|172\.(1[6-9]|2[0-9]|3[01])|192\.168)\.[0-9]+\.[0-9]+`)

func digest(word string) string {
	sum := sha256.Sum256([]byte(word))
	return hex.EncodeToString(sum[:])[:16]
}

// tokens splits text into lower-case words, also at camelCase boundaries.
func tokens(text string) []string {
	var out []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			out = append(out, strings.ToLower(string(current)))
			current = current[:0]
		}
	}
	var previous rune
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if unicode.IsUpper(r) && unicode.IsLower(previous) {
				flush()
			}
			current = append(current, r)
		default:
			flush()
		}
		previous = r
	}
	flush()
	return out
}

// Refused reports the first word of text that must not appear, or "".
func Refused(text string) string {
	for _, token := range tokens(text) {
		if _, bad := words[digest(token)]; bad {
			return token
		}
		if trimmed := strings.TrimSuffix(token, "s"); trimmed != token {
			if _, bad := words[digest(trimmed)]; bad {
				return token
			}
		}
		for _, length := range fragmentLengths {
			for start := 0; start+length <= len(token); start++ {
				if _, bad := fragments[digest(token[start:start+length])]; bad {
					return token
				}
			}
		}
	}
	return privateAddress.FindString(text)
}
