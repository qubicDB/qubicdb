// Text cleaning pipeline for embedding pre-processing.
// Strips HTML/XML tags, custom markup, emoji, control characters, and
// normalises whitespace before text is passed to the embedding model.

package vector

import (
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

var stripPolicy = bluemonday.StripTagsPolicy()

// CleanText runs the full cleaning pipeline on raw input text:
//  1. Strip HTML / XML tags, inserting spaces between adjacent text nodes
//  2. Remove emoji and non-printable / control characters
//  3. Collapse whitespace and trim
func CleanText(text string) string {
	text = stripHTMLWithSpaces(text)
	text = removeNonPrintable(text)
	text = collapseWhitespace(text)
	return text
}

// skipTags lists HTML tags whose text content must be discarded entirely.
var skipTags = map[string]bool{
	"script": true,
	"style":  true,
	"head":   true,
}

// stripHTMLWithSpaces tokenizes HTML and joins text nodes with spaces so that
// adjacent block-level tags do not produce run-together words.
// Content inside script, style, and head tags is discarded.
func stripHTMLWithSpaces(text string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(text))
	var b strings.Builder
	b.Grow(len(text))
	depth := 0 // nesting depth inside a skip tag
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			if skipTags[string(name)] {
				depth++
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			if skipTags[string(name)] && depth > 0 {
				depth--
			}
		case html.TextToken:
			if depth > 0 {
				continue
			}
			t := string(tokenizer.Text())
			if strings.TrimSpace(t) != "" {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

// removeNonPrintable drops every rune that is:
//   - a Unicode control character (Cc category, covers ASCII 0-31 and 127)
//   - an emoji or pictographic symbol (So, Sm, Sk, Cs categories + emoji ranges)
//   - a private-use or surrogate code point
//
// Printable letters, digits, punctuation, and common symbols are kept.
func removeNonPrintable(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if keepRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func keepRune(r rune) bool {
	if r == '\n' || r == '\r' || r == '\t' {
		return true
	}
	// Drop control characters (Cc)
	if unicode.Is(unicode.Cc, r) {
		return false
	}
	// Drop Unicode variation selectors (FE00–FE0F, FE10–FE1F)
	if r >= 0xFE00 && r <= 0xFE1F {
		return false
	}
	// Drop surrogates and private-use areas
	if r >= 0xD800 && r <= 0xDFFF {
		return false
	}
	if r >= 0xE000 && r <= 0xF8FF {
		return false
	}
	if r >= 0xF0000 {
		return false
	}
	// Drop emoji and pictographic blocks:
	//   Emoticons:                 1F600–1F64F
	//   Misc Symbols & Pictographs:1F300–1F5FF
	//   Transport & Map:           1F680–1F6FF
	//   Supplemental Symbols:      1F900–1F9FF
	//   Symbols & Pictographs Ext: 1FA00–1FA6F, 1FA70–1FAFF
	//   Dingbats:                  2702–27B0
	//   Misc Symbols:              2600–26FF
	//   Enclosed Alphanumeric Sup: 1F100–1F1FF
	if (r >= 0x1F600 && r <= 0x1F64F) ||
		(r >= 0x1F300 && r <= 0x1F5FF) ||
		(r >= 0x1F680 && r <= 0x1F6FF) ||
		(r >= 0x1F900 && r <= 0x1F9FF) ||
		(r >= 0x1FA00 && r <= 0x1FAFF) ||
		(r >= 0x2702 && r <= 0x27B0) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x1F100 && r <= 0x1F1FF) {
		return false
	}
	return true
}

// collapseWhitespace replaces runs of whitespace (including newlines and tabs)
// with a single space and trims leading/trailing whitespace.
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}
