package util

import (
	"encoding/json"
	"errors"
)

type SelectorElement struct {
	String string
	Group  []string
	Any    bool
}

func (se *SelectorElement) UnmarshalJSON(input []byte) error {
	if input[0] == '"' {
		if err := json.Unmarshal(input, &se.String); err != nil {
			return err
		}

		if se.String == "*" {
			se.Any = true
			se.String = ""
		}

		return nil
	}

	if input[0] == '[' {
		return json.Unmarshal(input, &se.Group)
	}

	return errors.New("the Go SelectorElement type can only be a string or an array of strings")
}

func (se *SelectorElement) MarshalJSON() ([]byte, error) {
	if se.Any {
		return []byte("\"*\""), nil
	}

	if se.String != "" {
		return json.Marshal(se.String)
	}

	if se.Group != nil {
		return json.Marshal(se.Group)
	}

	return nil, errors.New("a Go Selector must be a non-empty string or a non-empty slice of strings")
}

type Selector []SelectorElement
