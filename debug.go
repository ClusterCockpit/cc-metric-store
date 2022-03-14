package main

import (
	"bufio"
	"fmt"
)

func (b *buffer) debugDump(w *bufio.Writer) {
	if b.prev != nil {
		b.prev.debugDump(w)
	}

	end := ""
	if b.next != nil {
		end = " -> "
	}

	to := b.start + b.frequency*int64(len(b.data))
	fmt.Fprintf(w, "buffer(from=%d, len=%d, to=%d, archived=%v)%s", b.start, len(b.data), to, b.archived, end)
}

func (l *level) debugDump(w *bufio.Writer, m *MemoryStore, indent string) error {
	l.lock.RLock()
	defer l.lock.RUnlock()

	for name, minfo := range m.metrics {
		b := l.metrics[minfo.offset]
		if b != nil {
			fmt.Fprintf(w, "%smetric '%s': ", indent, name)
			b.debugDump(w)
			fmt.Fprint(w, "\n")
		}
	}

	if l.children != nil && len(l.children) > 0 {
		fmt.Fprintf(w, "%schildren:\n", indent)
		for name, lvl := range l.children {
			fmt.Fprintf(w, "%s'%s':\n", indent, name)
			lvl.debugDump(w, m, "\t"+indent)
		}
	}

	return nil
}

func (m *MemoryStore) DebugDump(w *bufio.Writer) error {
	fmt.Fprintf(w, "MemoryStore (%d MB):\n", m.SizeInBytes()/1024/1024)
	m.root.debugDump(w, m, "  ")
	return w.Flush()
}
