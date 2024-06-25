package main

import (
	"encoding/json"
	"errors"
	"log"
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

	log.Printf(">>>> HELLO FINDBUFFERS <<<<")
	log.Printf(">>>> FINDBUFFERS : FOR SELECTOR >>>> %v", selector)
	// log.Printf(">>>> FINDBUFFERS : FOR OFFSET   >>>> %d", offset)

	if len(selector) == 0 {
		b := l.metrics[offset]
		if b != nil {
			log.Printf(">>>> FINDBUFFERS : SELECTOR ARRAY EMPTY      >>>> FOUND BUFFER AT METRICS OFFSET %d", offset)
			log.Printf(">>>> FINDBUFFERS : END RECURSIVE FINDBUFFERS >>>> RETURN BUFFER")
			return f(b)
		}

		for _, lvl := range l.children {
			log.Printf(">>>> FINDBUFFERS : SELECTOR ARRAY EMPTY >>>> ITERATING LEVEL >>>> %v", lvl)
			err := lvl.findBuffers(nil, offset, f)
			if err != nil {
				return err
			}
		}

		log.Printf(">>>> FINDBUFFERS : SELECTOR ARRAY EMPTY      >>>> NO BUFFER MATCHED")
		log.Printf(">>>> FINDBUFFERS : END RECURSIVE FINDBUFFERS >>>> RETURN NIL")
		return nil
	}

	sel := selector[0]

	log.Printf(">>>> FINDBUFFERS : LEVEL CHILDREN >>>> NOT NIL : %v", (l.children != nil))
	// log.Printf(">>>> FINDBUFFERS : SELECTOR ELEMENT @ INDEX 0 >>>> %v", sel)

	log.Printf(">>>> FINDBUFFERS : SELECTOR ELEMENT >>>> STRING : %s", sel.String)
	if len(sel.String) != 0 && l.children != nil {
		// log.Printf(">>>> FINDBUFFERS : NAMED SELECTOR && CHILDREN AVAILABLE")
		lvl, ok := l.children[sel.String]
		if ok {
			log.Printf(">>>> FINDBUFFERS : NAMED >>>> LOOKUP CHILDREN")
			log.Printf(">>>> FINDBUFFERS : NAMED >>>> DO RECURSIVE FINDBUFFERS")
			err := lvl.findBuffers(selector[1:], offset, f)
			if err != nil {
				log.Printf(">>>> FINDBUFFERS : NAMED >>>> RECURSIVE ERROR >>>> $s", err)
				return err
			}
		}
		return nil
	}

	log.Printf(">>>> FINDBUFFERS : SELECTOR ELEMENT >>>> GROUP : %v", sel.Group)
	if sel.Group != nil && l.children != nil {
		// log.Printf(">>>> FINDBUFFERS : GROUP && CHILDREN AVAILABLE")
		for _, key := range sel.Group {
			lvl, ok := l.children[key]
			if ok {
				log.Printf(">>>> FINDBUFFERS : GROUP >>>> LOOKUP CHILDREN FOR GROUPKEY >>>> %s", key)
				log.Printf(">>>> FINDBUFFERS : GROUP >>>> DO RECURSIVE FINDBUFFERS")
				err := lvl.findBuffers(selector[1:], offset, f)
				if err != nil {
					log.Printf(">>>> FINDBUFFERS : GROUP >>>> RECURSIVE ERROR >>>> $s", err)
					return err
				}
			}
		}
		log.Printf(">>>> FINDBUFFERS : GROUP >>>> RETURN NIL")
		return nil
	}

	log.Printf(">>>> FINDBUFFERS : SELECTOR ELEMENT >>>> IS ANY : %v", sel.Any)
	if sel.Any && l.children != nil {
		// log.Printf(">>>> FINDBUFFERS : ENABLED ANY && CHILDREN AVAILABLE")
		for _, lvl := range l.children {
			log.Printf(">>>> FINDBUFFERS : ANY >>>> DO RECURSIVE FINDBUFFERS")
			if err := lvl.findBuffers(selector[1:], offset, f); err != nil {
				log.Printf(">>>> FINDBUFFERS : ANY >>>> RECURSIVE ERROR >>>> $s", err)
				return err
			}
		}
		log.Printf(">>>> FINDBUFFERS : ANY >>>> RETURN NIL")
		return nil
	}

	log.Printf(">>>> FINDBUFFERS : NO MATCHED CASE >>>> RETURN NIL")
	return nil
}
