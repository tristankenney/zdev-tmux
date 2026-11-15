package render

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

// FitHeight fits a Body-shaped frame into maxLines, pinning the head and
// footer and scrolling the rows between so focusY stays visible. prevTop is
// the previous call's returned top, which keeps scrolling minimal.
func FitHeight(body []byte, rows []RowRef, maxLines, focusY, prevTop int) ([]byte, []RowRef, int) {
	lines := bytes.SplitAfter(body, []byte("\n"))
	if n := len(lines); n > 0 && len(lines[n-1]) == 0 {
		lines = lines[:n-1]
	}
	n := len(lines)
	if maxLines <= 0 || n <= maxLines {
		return body, rows, 0
	}

	hasRef := make(map[int]bool, len(rows))
	for _, r := range rows {
		hasRef[r.Y] = true
	}
	head := 1
	for y := 0; y < 2 && y < n; y++ {
		if !hasRef[y] {
			head = y + 1
			break
		}
	}
	foot := 1
	avail := maxLines - head - foot
	if avail < 3 {
		head, foot, avail = 0, 0, maxLines
	}
	mid := n - head - foot

	top := clampInt(prevTop, 0, mid-avail)
	if f := focusY - head; focusY >= 0 && f >= 0 && f < mid {
		lo := top
		if top > 0 {
			lo++
		}
		hi := top + avail - 1
		if top+avail < mid {
			hi--
		}
		switch {
		case f < lo:
			top = clampInt(f-1, 0, mid-avail)
		case f > hi:
			top = clampInt(f-avail+2, 0, mid-avail)
		}
	}
	upMarker := top > 0
	downMarker := top+avail < mid

	var out bytes.Buffer
	var outRows []RowRef
	emit := func(srcY int) {
		for _, r := range rows {
			if r.Y == srcY {
				r.Y = bytes.Count(out.Bytes(), []byte("\n"))
				outRows = append(outRows, r)
			}
		}
		out.Write(lines[srcY])
	}
	for y := 0; y < head; y++ {
		emit(y)
	}
	first, last := top, top+avail
	if upMarker {
		writeMoreLine(&out, "↑", top+1)
		first++
	}
	if downMarker {
		last--
	}
	for y := first; y < last; y++ {
		emit(head + y)
	}
	if downMarker {
		writeMoreLine(&out, "↓", mid-last)
	}
	for y := n - foot; y < n; y++ {
		emit(y)
	}
	return out.Bytes(), outRows, top
}

func writeMoreLine(buf *bytes.Buffer, arrow string, count int) {
	buf.WriteString("  ")
	buf.WriteString(thDim())
	buf.WriteString(fmt.Sprintf("%s %d more", arrow, count))
	buf.WriteString(Reset)
	buf.WriteByte('\n')
}

func FocusLine(snap *proto.Snapshot, rows []RowRef) int {
	if snap == nil {
		return -1
	}
	if snap.CursorActive {
		flat := proto.FlatRows(snap, teamRowsFor(snap))
		if snap.CursorRow >= 0 && snap.CursorRow < len(flat) {
			want := flat[snap.CursorRow]
			for _, r := range rows {
				if r.Name == want.SwitchTo && r.WindowID == want.WindowID {
					return r.Y
				}
			}
		}
	}
	if snap.CurrentSession == "" {
		return -1
	}
	for _, r := range rows {
		if r.WindowID == "" && strings.EqualFold(proto.SessionKey(r.Name), snap.CurrentSession) {
			return r.Y
		}
	}
	return -1
}

func clampInt(v, lo, hi int) int {
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}
