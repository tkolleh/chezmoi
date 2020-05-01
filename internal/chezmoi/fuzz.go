// +build gofuzz

package chezmoi

import (
	"bytes"
	"text/template"
)

const (
	uninteresting = -1
	expected      = 0
	interesting   = 1
)

func Fuzz(data []byte) (result int) {
	t, err := template.New("").Parse(string(data))
	if err != nil {
		// go-fuzz generated an invalid template, of which there are many.
		return uninteresting
	}

	b := &bytes.Buffer{}
	if err := t.Execute(b, nil); err != nil {
		// go-fuzz generated a template that does not execute, of which there
		// are many.
		return uninteresting
	}

	if bytes.Equal(data, b.Bytes()) {
		// The template is valid and the generated output matches the input.
		return expected
	}

	// The template, when executed, generated output that was not equal to
	// the input.
	return interesting
}
