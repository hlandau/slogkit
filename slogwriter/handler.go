// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slogwriter

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	_ "unsafe"

	"github.com/hlandau/slogkit/slogwriter/internal/buffer"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

type defaultHandler struct {
	ch *commonHandler
	// log.Output, except for testing
	output func(calldepth int, message string) error
}

func newDefaultHandler(output func(int, string) error) *defaultHandler {
	return &defaultHandler{
		ch:     &commonHandler{json: false},
		output: output,
	}
}

func (*defaultHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= slog.LevelInfo
}

// Collect the level, attributes and message in a string and
// write it with the default log.Logger.
// Let the log.Logger handle time and file/line.
func (h *defaultHandler) Handle(ctx context.Context, r slog.Record) error {
	buf := buffer.New()
	buf.WriteString(r.Level.String())
	buf.WriteByte(' ')
	buf.WriteString(r.Message)
	state := h.ch.newHandleState(buf, true, " ", nil)
	defer state.free()
	state.appendNonBuiltIns(r)

	// skip [h.output, defaultHandler.Handle, handlerWriter.Write, log.Output]
	return h.output(4, buf.String())
}

func (h *defaultHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &defaultHandler{h.ch.withAttrs(as), h.output}
}

func (h *defaultHandler) WithGroup(name string) slog.Handler {
	return &defaultHandler{h.ch.withGroup(name), h.output}
}

type commonHandler struct {
	json              bool // true => output JSON; false => output text
	opts              HandlerOptions
	preformattedAttrs []byte
	groupPrefix       string   // for text: prefix of groups opened in preformatting
	groups            []string // all groups started from WithGroup
	nOpenGroups       int      // the number of groups opened in preformattedAttrs
	mu                sync.Mutex
	w                 io.Writer
}

func (h *commonHandler) clone() *commonHandler {
	// We can't use assignment because we can't copy the mutex.
	return &commonHandler{
		json:              h.json,
		opts:              h.opts,
		preformattedAttrs: slices.Clip(h.preformattedAttrs),
		groupPrefix:       h.groupPrefix,
		groups:            slices.Clip(h.groups),
		nOpenGroups:       h.nOpenGroups,
		w:                 h.w,
	}
}

// enabled reports whether l is greater than or equal to the
// minimum level.
func (h *commonHandler) enabled(l slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return l >= minLevel
}

//go:linkname recordSource golang.org/x/exp/slog.Record.source
func recordSource(r slog.Record) *slog.Source

func recordSourceEx(r slog.Record) *slog.Source {
	s := recordSource(r)
	s.File = filepath.Base(s.File)
	return s
}

func (h *commonHandler) withAttrs(as []slog.Attr) *commonHandler {
	h2 := h.clone()
	// Pre-format the attributes as an optimization.
	prefix := buffer.New()
	defer prefix.Free()
	prefix.WriteString(h.groupPrefix)
	state := h2.newHandleState((*buffer.Buffer)(&h2.preformattedAttrs), false, "", prefix)
	defer state.free()
	if len(h2.preformattedAttrs) > 0 {
		state.sep = h.attrSep()
	}
	state.openGroups()
	for _, a := range as {
		state.appendAttr(a)
	}
	// Remember the new prefix for later keys.
	h2.groupPrefix = state.prefix.String()
	// Remember how many opened groups are in preformattedAttrs,
	// so we don't open them again when we handle a Record.
	h2.nOpenGroups = len(h2.groups)
	return h2
}

func (h *commonHandler) withGroup(name string) *commonHandler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

func (h *commonHandler) handle(ctx context.Context, r slog.Record) error {
	state := h.newHandleState(buffer.New(), true, "", nil)
	defer state.free()
	if h.json {
		state.buf.WriteByte('{')
	}
	// Built-in attributes. They are not in a group.
	stateGroups := state.groups
	state.groups = nil // So ReplaceAttrs sees no groups instead of the pre groups.
	rep := h.opts.ReplaceAttr
	// time
	if !r.Time.IsZero() {
		key := slog.TimeKey
		val := r.Time.Round(0) // strip monotonic to match Attr behavior
		if rep == nil {
			if !h.opts.NoColor {
				state.buf.WriteString("\x1b[90m")
			}
			state.appendTime(val)
			if !h.opts.NoColor {
				state.buf.WriteString("\x1b[0m")
			}
		} else {
			state.appendAttrEx(slog.Time(key, val), 1)
		}
	}
	// level
	key := slog.LevelKey
	val := r.Level
	if rep == nil {
		state.buf.WriteString(" ")
		if !h.opts.NoColor {
			if val >= slog.LevelError {
				state.buf.WriteString("\x1b[91m")
			} else if val >= slog.LevelWarn {
				state.buf.WriteString("\x1b[93m")
			} else {
				state.buf.WriteString("\x1b[0m")
			}
		}
		state.appendString(val.String()[0:3])
	} else {
		if !h.opts.NoColor {
			state.buf.WriteString("\x1b[1m")
		}
		state.appendAttrEx(slog.Any(key, val), 2)
	}
	if !h.opts.NoColor {
		state.buf.WriteString("\x1b[0m")
	}
	key = slog.MessageKey
	msg := r.Message
	if rep == nil {
		state.buf.WriteString(" ")
		if !h.opts.NoColor {
			state.buf.WriteString("\x1b[1m")
		}
		state.appendString(msg)
		if !h.opts.NoColor {
			state.buf.WriteString("\x1b[0m")
		}
	} else {
		state.appendAttrEx(slog.String(key, msg), 3)
	}
	state.groups = stateGroups // Restore groups passed to ReplaceAttrs.
	state.sep = h.attrSep()
	state.appendNonBuiltIns(r)
	// source
	if h.opts.AddSource {
		if !h.opts.NoColor {
			state.buf.WriteString("\x1b[90m")
		}
		state.appendAttrEx(slog.Any(slog.SourceKey, recordSourceEx(r)), 2)
		if !h.opts.NoColor {
			state.buf.WriteString("\x1b[0m")
		}
	}
	state.buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	var err error
	if h.opts.WriterFunc != nil {
		err = h.opts.WriterFunc(ctx, *state.buf, r)
	} else {
		_, err = h.w.Write(*state.buf)
	}
	return err
}

func (s *handleState) appendNonBuiltIns(r slog.Record) {
	// preformatted Attrs
	if len(s.h.preformattedAttrs) > 0 {
		s.buf.WriteString(s.sep)
		s.buf.Write(s.h.preformattedAttrs)
		s.sep = s.h.attrSep()
	}
	// Attrs in Record -- unlike the built-in ones, they are in groups started
	// from WithGroup.
	s.prefix = buffer.New()
	defer s.prefix.Free()
	s.prefix.WriteString(s.h.groupPrefix)
	s.openGroups()
	r.Attrs(func(a slog.Attr) bool {
		s.appendAttr(a)
		return true
	})
	if s.h.json {
		// Close all open groups.
		for range s.h.groups {
			s.buf.WriteByte('}')
		}
		// Close the top-level object.
		s.buf.WriteByte('}')
	}
}

// attrSep returns the separator between attributes.
func (h *commonHandler) attrSep() string {
	if h.json {
		return ","
	}
	return " "
}

// handleState holds state for a single call to commonHandler.handle.
// The initial value of sep determines whether to emit a separator
// before the next key, after which it stays true.
type handleState struct {
	h       *commonHandler
	buf     *buffer.Buffer
	freeBuf bool           // should buf be freed?
	sep     string         // separator to write before next key
	prefix  *buffer.Buffer // for text: key prefix
	groups  *[]string      // pool-allocated slice of active groups, for ReplaceAttr
}

var groupPool = sync.Pool{New: func() any {
	s := make([]string, 0, 10)
	return &s
}}

func (h *commonHandler) newHandleState(buf *buffer.Buffer, freeBuf bool, sep string, prefix *buffer.Buffer) handleState {
	s := handleState{
		h:       h,
		buf:     buf,
		freeBuf: freeBuf,
		sep:     sep,
		prefix:  prefix,
	}
	if h.opts.ReplaceAttr != nil {
		s.groups = groupPool.Get().(*[]string)
		*s.groups = append(*s.groups, h.groups[:h.nOpenGroups]...)
	}
	return s
}

func (s *handleState) free() {
	if s.freeBuf {
		s.buf.Free()
	}
	if gs := s.groups; gs != nil {
		*gs = (*gs)[:0]
		groupPool.Put(gs)
	}
}

func (s *handleState) openGroups() {
	for _, n := range s.h.groups[s.h.nOpenGroups:] {
		s.openGroup(n)
	}
}

// Separator for group names and keys.
const keyComponentSep = '.'

// openGroup starts a new group of attributes
// with the given name.
func (s *handleState) openGroup(name string) {
	if s.h.json {
		s.appendKey(name)
		s.buf.WriteByte('{')
		s.sep = ""
	} else {
		s.prefix.WriteString(name)
		s.prefix.WriteByte(keyComponentSep)
	}
	// Collect group names for ReplaceAttr.
	if s.groups != nil {
		*s.groups = append(*s.groups, name)
	}
}

// closeGroup ends the group with the given name.
func (s *handleState) closeGroup(name string) {
	if s.h.json {
		s.buf.WriteByte('}')
	} else {
		(*s.prefix) = (*s.prefix)[:len(*s.prefix)-len(name)-1 /* for keyComponentSep */]
	}
	s.sep = s.h.attrSep()
	if s.groups != nil {
		*s.groups = (*s.groups)[:len(*s.groups)-1]
	}
}

func attrIsEmpty(a slog.Attr) bool {
	return a.Key == "" && a.Value.Kind() == slog.KindAny && a.Value.Any() == nil
}

func sourceGroup(s *slog.Source) slog.Value {
	var as []slog.Attr
	if s.Function != "" {
		as = append(as, slog.String("function", s.Function))
	}
	if s.File != "" {
		as = append(as, slog.String("file", s.File))
	}
	if s.Line != 0 {
		as = append(as, slog.Int("line", s.Line))
	}
	return slog.GroupValue(as...)
}

// appendAttr appends the Attr's key and value using app.
// It handles replacement and checking for an empty key.
// after replacement).
func (s *handleState) appendAttr(a slog.Attr) {
	s.appendAttrEx(a, 0)
}

func (s *handleState) appendAttrEx(a slog.Attr, flag int) {
	if rep := s.h.opts.ReplaceAttr; rep != nil && a.Value.Kind() != slog.KindGroup {
		var gs []string
		if s.groups != nil {
			gs = *s.groups
		}
		// Resolve before calling ReplaceAttr, so the user doesn't have to.
		a.Value = a.Value.Resolve()
		a = rep(gs, a)
	}
	a.Value = a.Value.Resolve()
	// Elide empty Attrs.
	if attrIsEmpty(a) {
		return
	}
	// Special case: Source.
	if v := a.Value; v.Kind() == slog.KindAny {
		if src, ok := v.Any().(*slog.Source); ok {
			if s.h.json {
				a.Value = sourceGroup(src)
			} else {
				a.Value = slog.StringValue(fmt.Sprintf("%s:%d", src.File, src.Line))
			}
		}
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		// Output only non-empty groups.
		if len(attrs) > 0 {
			// Inline a group with an empty key.
			if a.Key != "" {
				s.openGroup(a.Key)
			}
			for _, aa := range attrs {
				s.appendAttr(aa)
			}
			if a.Key != "" {
				s.closeGroup(a.Key)
			}
		}
	} else {
		if flag == 0 {
			s.appendKey(a.Key)
		} else if flag == 2 {
			s.buf.WriteString(" <")
		} else {
			s.buf.WriteString(" ")
		}
		s.appendValue(a.Value)
		if flag == 2 {
			s.buf.WriteString(">")
		}
	}
}

func (s *handleState) appendError(err error) {
	s.appendString(fmt.Sprintf("!ERROR:%v", err))
}

func (s *handleState) appendKey(key string) {
	s.buf.WriteString(s.sep)
	if s.prefix != nil {
		// TODO: optimize by avoiding allocation.
		s.appendString(string(*s.prefix) + key)
	} else {
		s.appendString(key)
	}
	if s.h.json {
		s.buf.WriteByte(':')
	} else {
		s.buf.WriteByte('=')
	}
	s.sep = s.h.attrSep()
}

func (s *handleState) appendString(str string) {
	if s.h.json {
		s.buf.WriteByte('"')
		*s.buf = appendEscapedJSONString(*s.buf, str)
		s.buf.WriteByte('"')
	} else {
		// text
		if needsQuoting(str) {
			*s.buf = strconv.AppendQuote(*s.buf, str)
		} else {
			s.buf.WriteString(str)
		}
	}
}

func (s *handleState) appendValue(v slog.Value) {
	var err error
	if s.h.json {
		err = appendJSONValue(s, v)
	} else {
		err = appendTextValue(s, v)
	}
	if err != nil {
		s.appendError(err)
	}
}

func (s *handleState) appendTime(t time.Time) {
	if s.h.json {
		appendJSONTime(s, t)
	} else {
		writeTimeRFC3339Millis(s.buf, t)
	}
}

// This takes half the time of Time.AppendFormat.
func writeTimeRFC3339Millis(buf *buffer.Buffer, t time.Time) {
	year, month, day := t.Date()
	buf.WritePosIntWidth(year, 4)
	buf.WriteByte('-')
	buf.WritePosIntWidth(int(month), 2)
	buf.WriteByte('-')
	buf.WritePosIntWidth(day, 2)
	buf.WriteByte('T')
	hour, min, sec := t.Clock()
	buf.WritePosIntWidth(hour, 2)
	buf.WriteByte(':')
	buf.WritePosIntWidth(min, 2)
	buf.WriteByte(':')
	buf.WritePosIntWidth(sec, 2)
	ns := t.Nanosecond()
	buf.WriteByte('.')
	buf.WritePosIntWidth(ns/1e6, 3)
	_, offsetSeconds := t.Zone()
	if offsetSeconds == 0 {
		buf.WriteByte('Z')
	} else {
		offsetMinutes := offsetSeconds / 60
		if offsetMinutes < 0 {
			buf.WriteByte('-')
			offsetMinutes = -offsetMinutes
		} else {
			buf.WriteByte('+')
		}
		buf.WritePosIntWidth(offsetMinutes/60, 2)
		buf.WriteByte(':')
		buf.WritePosIntWidth(offsetMinutes%60, 2)
	}
}
