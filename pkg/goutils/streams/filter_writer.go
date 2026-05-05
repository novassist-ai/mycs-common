package streams

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
)

const (
	undefined int = iota
	passThru
	blackHole
	applyFilters
)

const (
	stateInclude int = iota
	stateExclude
)

// Filter
type Filter struct {
	filterType int
	state      int

	excludeAfterPatterns,
	includeAfterPatterns,
	excludePatterns,
	includePatterns []*regexp.Regexp

	hasExcludeAfterPatterns,
	hasIncludeAfterPatterns,
	hasExcludePatterns,
	hasIncludePatterns bool
}

// transforming filter that can be used to
// filter text output written to a io.Writer.
//
// implements golang/text/transform.Transformer
type filterWriter struct {
	outputBuffer io.Writer
	currentLine  bytes.Buffer

	filter *Filter
}

func NewFilterWriter(
	filter *Filter,
	outputBuffer io.Writer,
) io.WriteCloser {

	return &filterWriter{
		outputBuffer: outputBuffer,
		filter:       filter,
	}
}

func (f *Filter) ClearPatterns() {
	f.filterType = passThru
	f.state = stateInclude

	f.excludeAfterPatterns = []*regexp.Regexp{}
	f.includeAfterPatterns = []*regexp.Regexp{}
	f.excludePatterns = []*regexp.Regexp{}
	f.includePatterns = []*regexp.Regexp{}
}

func (f *Filter) SetPassThru() {
	f.filterType = passThru
	f.state = stateInclude
}

func (f *Filter) SetBlackHole() {
	f.filterType = blackHole
	f.state = stateExclude
}

func (f *Filter) AddExcludeAfterPattern(pattern string) {
	f.filterType = applyFilters
	if len(f.includeAfterPatterns) == 0 {
		f.state = stateInclude
	}
	f.excludeAfterPatterns = append(f.excludeAfterPatterns, regexp.MustCompile(pattern))
	f.hasExcludeAfterPatterns = true
}

func (f *Filter) AddIncludeAfterPattern(pattern string) {
	f.filterType = applyFilters
	if len(f.excludeAfterPatterns) == 0 {
		f.state = stateExclude
	}
	f.includeAfterPatterns = append(f.includeAfterPatterns, regexp.MustCompile(pattern))
	f.hasIncludeAfterPatterns = true
}

func (f *Filter) AddExcludePattern(pattern string) {
	f.filterType = applyFilters
	f.excludePatterns = append(f.excludePatterns, regexp.MustCompile(pattern))
	f.hasExcludePatterns = true
}

func (f *Filter) AddIncludePattern(pattern string) {
	f.filterType = applyFilters
	f.includePatterns = append(f.includePatterns, regexp.MustCompile(pattern))
	f.hasIncludePatterns = true
}

func (f *Filter) isMatch(patterns []*regexp.Regexp, line []byte) bool {
	for _, re := range patterns {
		if re.Match(line) {
			return true
		}
	}
	return false
}

// interface: io.Writer

func (w *filterWriter) Write(data []byte) (int, error) {

	var (
		err error
	)

	switch w.filter.filterType {

	case blackHole:
		return len(data), nil

	case passThru:
		return w.outputBuffer.Write(data)

	case applyFilters:
		for i, b := range data {
			w.currentLine.WriteByte(b)

			if b == '\n' {
				buffer := w.currentLine.Bytes()
				matchBuffer := buffer[0 : w.currentLine.Len()-1]
				w.currentLine.Reset()

				if w.filter.state == stateInclude {
					if (!w.filter.hasIncludePatterns || w.filter.isMatch(w.filter.includePatterns, matchBuffer)) &&
						(!w.filter.hasExcludePatterns || !w.filter.isMatch(w.filter.excludePatterns, matchBuffer)) {
						if _, err = w.outputBuffer.Write(buffer); err != nil {
							return 0, err
						}
					}

					if w.filter.isMatch(w.filter.excludeAfterPatterns, matchBuffer) {
						w.filter.state = stateExclude
						if !w.filter.hasIncludeAfterPatterns {
							// if there are no patterns to include back the
							// stream of data then the remaining data should
							// be sent to a blackhole
							w.filter.filterType = blackHole
							break
						}
					}

				} else {
					// stateExclude
					if w.filter.isMatch(w.filter.includeAfterPatterns, matchBuffer) {
						w.filter.state = stateInclude
						if !w.filter.hasExcludeAfterPatterns {
							// if there are no patterns to exclude lines from
							// remainder of stream then the remaining data
							// should be treated as pass thru.
							w.filter.filterType = passThru
							if _, err = w.outputBuffer.Write(data[i+1:]); err != nil {
								return 0, nil
							}
							break
						}
					}
				}
			}
		}
		return len(data), nil
	}

	return 0, fmt.Errorf("unknown filter type")
}

// interface: io.Writer

func (w *filterWriter) Close() error {

	var (
		err error
	)

	if w.filter.state == stateInclude {
		// flush any remaining data to output. this can
		// happen if last line needs to be included but
		// does not end in a new-line.
		if _, err = w.outputBuffer.Write(w.currentLine.Bytes()); err != nil {
			return nil
		}
	}
	w.filter.filterType = undefined
	return nil
}
