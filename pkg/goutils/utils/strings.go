package utils

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math/rand"
	"regexp"
	"strings"

	"github.com/minio/highwayhash"
)

var hashKey []byte

func init() {
	// initialize hash key to a value
	// which will always be the same
	hashKey = make([]byte, 32)
	for i := 0; i < 32; i++ {
    hashKey[i] = byte(i*8)
	}
}

func PtrToStr(s string) *string {
	return &s
}

func CopyStrPtr(s *string) *string {
	if s != nil {
		copy := *s
		return &copy
	} else {
		return nil
	}
}

func JoinListAsSentence(format string, list []string, quoteListItems bool) string {

	var (
		listAsString strings.Builder
	)

	joinItem := func(item string) {

		if quoteListItems {
			listAsString.WriteByte('"')
		}
		listAsString.WriteString(item)
		if quoteListItems {
			listAsString.WriteByte('"')
		}
	}

	l := len(list)
	if l > 0 {
		l--

		for i, v := range list {

			if i == 0 {
				joinItem(v)
			} else {
				if i == l {
					listAsString.WriteString(" and ")
				} else {
					listAsString.WriteString(", ")
				}
				joinItem(v)
			}
		}
	}

	return fmt.Sprintf(format, listAsString.String())
}

func SplitString(input string, indent, width int) []string {

	var (
		lines []string

		ch          byte
		currentLine string

		testWidth, 
		lastLine, 
		lineLength, 
		splitAt, 
		nextAt int
	)

	lastLine = 0
	lines = []string{input}

	isWhitespace := func(ch byte) bool {
		return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
	}

	if indent > 0 {
		testWidth = width + indent			
	} else {
		testWidth = width
	}
	for len(lines[lastLine]) > testWidth {

		splitAt = testWidth
		testWidth = width
		currentLine = lines[lastLine]
		lineLength = len(currentLine)

		// Find first white space char before or at `width`
		ch = currentLine[splitAt]
		for splitAt > 0 && !isWhitespace(ch) {
			splitAt--
			ch = currentLine[splitAt]
		}
		if splitAt > 0 {

			// Find next non-whitespace char to break out new line
			// This loop will be invoked only if `width` happens to
			// fall within a sequence of whitespaces
			nextAt = splitAt
			ch = ' '
			for isWhitespace(ch) {
				nextAt++
				if nextAt == lineLength {
					break
				}
				ch = currentLine[nextAt]
			}

			// Find non-whitespace character where the current line
			// should break at. This loop will be invoked only if
			// there exists a sequence of whitespaces before `splitAt`.
			ch = currentLine[splitAt]
			for splitAt > 0 && isWhitespace(ch) {
				splitAt--
				ch = currentLine[splitAt]
			}

			if splitAt == 0 {
				// Current line to break is a cotiguous sequence of
				// white spaces
				if nextAt < lineLength {
					lines[lastLine] = currentLine[nextAt:]
				}
			} else {
				// Split current line
				lines[lastLine] = currentLine[:splitAt+1]

				// Break out next line
				if nextAt < lineLength {
					lines = append(lines, currentLine[nextAt:])
					lastLine++
				}
			}

		} else {
			// Break line exactly at width as line is a contiguos
			// sequence of non-whitespace characters.
			lines = append(lines, currentLine[width:])
			lines[lastLine] = currentLine[:width]
			lastLine++
		}
	}

	return lines
}

func FormatMultilineString(
	input string, 
	indent, width int, 
	padFirstIndent bool,
	splitEven bool,
) (string, bool) {

	var (
		lines    []string
		lastLine int

		out strings.Builder
	)

	if padFirstIndent || splitEven {
		lines = SplitString(input, 0, width)
	} else {
		lines = SplitString(input, indent, width - indent)
	}
	lastLine = len(lines) - 1
	for i, l := range lines {
		if indent > 0 && (padFirstIndent || i > 0) {
			out.WriteString(strings.Repeat(" ", indent))
		}
		out.WriteString(l)
		if i != lastLine {
			out.WriteString("\n")
		}
	}
	return out.String(), len(lines) > 1
}

func FormatMessage(
	indent, width int,
	indentFirst, capitalize bool,
	format string, args ...interface{},
) string {

	if capitalize {
		// capitalize all string arguments
		for i, a := range args {
			if s, ok := a.(string); ok {
				b := []byte(s)
				if b[0] > 96 && b[0] < 123 {
					b[0] = b[0] ^ ('a' - 'A')
					args[i] = string(b)
				}
			}
		}
	}

	message, _ := FormatMultilineString(fmt.Sprintf(format, args...), indent, width, indentFirst, false)
	return message
}

func RepeatString(s string, n int, out io.Writer) {

	outSequence := []byte(s)
	for i := 0; i < n; i++ {
		if _, err := out.Write(outSequence); err != nil {
			panic(err)
		}
	}
}

func RandomString(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63() % int64(len(letterBytes))]
	}
	return string(b)
}

func HashString(s, prefix string) (string, error) {

	var (
		err  error
		hash hash.Hash
	)

	// hash the sgs key so we have a fixed name
	// of fixed length within limits for the vmap
	if hash, err = highwayhash.New64(hashKey); err == nil {
		_, err = io.Copy(hash, bytes.NewBuffer([]byte(s)))
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(hash.Sum(nil))), nil
}

func ExtractMatches(buffer []byte, patterns map[string]*regexp.Regexp) map[string][][]string {

	var (
		matches [][]string

		line   string
		exists bool
	)

	results := make(map[string][][]string)
	scanner := bufio.NewScanner(bytes.NewReader(buffer))
	for scanner.Scan() {
		line = scanner.Text()
		for name, pattern := range patterns {
			if matches = pattern.FindAllStringSubmatch(line, -1); len(matches) > 0 && len(matches[0]) > 0 {
				if _, exists = results[name]; !exists {
					results[name] = matches
				} else {
					results[name] = append(results[name], matches...)
				}
			}
		}
	}
	return results
}

// Return a formated full name
func FormatFullName(first, middle, family string) string {
	
	var (
		buffer strings.Builder
	)

	l := len(first)
	if l == 1 {
		buffer.WriteByte(first[0])
		buffer.WriteByte('.')
	} else if l > 1 {
		buffer.WriteString(first)
	}
	l = len(middle)
	if l > 0 {
		buffer.WriteByte(' ')
		if l == 1 {
			buffer.WriteByte(middle[0])
			buffer.WriteByte('.')	
		} else {
			buffer.WriteString(middle)
		}
	}
	if len(family) > 0 {
		buffer.WriteByte(' ')
		buffer.WriteString(family)
	}
	return buffer.String()
}

/*
	Convert byte size to bytes, kilobytes, megabytes, gigabytes, ...

	These utility functions convert a byte size to a human-readable string
	in either SI (decimal) or IEC (binary) format.

	Source: https://yourbasic.org/golang/formatting-byte-size-to-human-readable-format/
*/

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func ByteCountIEC(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}
