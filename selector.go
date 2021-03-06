package main

import (
	"encoding/json"
	"errors"
)

type SelectorElement struct {
	Any    bool
	String string
	Group  []string
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

func (l *level) findLevel(selector []string) *level {
	if len(selector) == 0 {
		return l
	}

	l.lock.RLock()
	defer l.lock.RUnlock()

	lvl := l.children[selector[0]]
	if lvl == nil {
		return nil
	}

	return lvl.findLevel(selector[1:])
}

func (l *level) findBuffers(selector Selector, offset int, f func(b *buffer) error) error {
	l.lock.RLock()
	defer l.lock.RUnlock()

	if len(selector) == 0 {
		b := l.metrics[offset]
		if b != nil {
			return f(b)
		}

		for _, lvl := range l.children {
			err := lvl.findBuffers(nil, offset, f)
			if err != nil {
				return err
			}
		}
		return nil
	}

	sel := selector[0]
	if len(sel.String) != 0 && l.children != nil {
		lvl, ok := l.children[sel.String]
		if ok {
			err := lvl.findBuffers(selector[1:], offset, f)
			if err != nil {
				return err
			}
		}
		return nil
	}

	if sel.Group != nil && l.children != nil {
		for _, key := range sel.Group {
			lvl, ok := l.children[key]
			if ok {
				err := lvl.findBuffers(selector[1:], offset, f)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	if sel.Any && l.children != nil {
		for _, lvl := range l.children {
			if err := lvl.findBuffers(selector[1:], offset, f); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}
