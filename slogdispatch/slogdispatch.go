// Package slogdispatch provides dispatch and routing utilities for slog.
//
// # Multi Handler
//
// Dispatches a log message to multiple slog.Handlers.
//
// # Contextual Handler
//
// The contextual handler utility allows you to dispatch to a slog.Handler in a
// dynamic and programmatic fashion based on the context.Context, or in other
// arbitrary ways.
//
// # Simple Context Handler Store
//
// A simple contextual resolver implementation which can associate a single
// slog.Handler with a context and which always dispatches to that handler,
// or to a default handler if none is set. Helper functions are provided
// to create modified contexts with slog attributes, groups, etc.
//
// # Router Handler
//
// Processes a sequence of predicate rules and dispatches to arbitrary
// handlers accordingly.
//
// # Default Handler
//
// Forwards all calls to the slog.Default() handler. The only reason to use this
// (rather than just using slog.Default() directly) is if you are configuring
// something to use the default handler and want it to update automatically
// if a program changes slog.Default().
package slogdispatch

import (
	"context"
	"github.com/KarpelesLab/weak"
	"golang.org/x/exp/slog"
	"sync"
	"sync/atomic"
)

// Multi Handler

type multiHandler struct {
	handlers []slog.Handler
}

// Creates a slog.Handler which dispatches to each handler in the slice passed.
func NewMultiHandler(handlers []slog.Handler) slog.Handler {
	return &multiHandler{
		handlers: handlers,
	}
}

func (mh *multiHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	for _, subh := range mh.handlers {
		if subh.Enabled(ctx, lvl) {
			return true
		}
	}
	return false
}

func (mh *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error

	for _, subh := range mh.handlers {
		if !subh.Enabled(ctx, r.Level) {
			continue
		}

		err := subh.Handle(ctx, r)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (mh *multiHandler) specialise(attrs []slog.Attr, groupName string) slog.Handler {
	newHandlers := make([]slog.Handler, len(mh.handlers))
	for i, subh := range mh.handlers {
		var nextAttrs []slog.Attr
		if i != len(mh.handlers)-1 {
			nextAttrs = make([]slog.Attr, len(attrs))
			copy(nextAttrs, attrs)
		}
		nsubh := subh
		if len(attrs) > 0 {
			nsubh = nsubh.WithAttrs(attrs)
		}
		if groupName != "" {
			nsubh = nsubh.WithGroup(groupName)
		}
		newHandlers[i] = nsubh
		attrs = nextAttrs
	}

	return &multiHandler{
		handlers: newHandlers,
	}
}

func (mh *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(mh.handlers) == 0 {
		return mh
	}

	return mh.specialise(attrs, "")
}

func (mh *multiHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return mh
	}

	return mh.specialise(nil, name)
}

// Contextual Handler

// A handler cache maps a slog.Handler (the base handler) to a set of derived
// handlers. This is needed to handle the slog.Handler.WithArgs and
// slog.Handler.WithGroup methods.
//
// A contextual resolver implements slog.Handler and thus can itself be
// derived using WithArgs and WithGroup. Because the handler to be dispatched
// to is only determined at actual logging time when a context is passed (and
// moreover since a context argument is not available for WithArgs/WithGroup),
// specialisation of a contextual resolver into a derived contextual resolver
// with the given data must be deferred until actual logging time. However,
// deriving such a logger for every log call would defeat the performance
// objectives of slog.Handler's WithArgs/WithGroup design.
//
// Therefore, a derived contextual handler uses a cache of derived
// slog.Handlers. For every handler h which might be yielded by a contextual
// resolver for some given context value, an associated cache for that handler
// h is maintained, which maps derived contextual handlers to handlers derived
// from the base actual handler.
//
// These derived handlers are constructed as needed internally and maintained
// in a weak map to ensure they are freed if not needed anymore.
type HandlerCache struct {
	handler slog.Handler
	cache   *weak.Map[uint64, slog.Handler]
}

// Create a new handler cache for the given base handler.
func NewHandlerCache(handler slog.Handler) *HandlerCache {
	return &HandlerCache{
		handler: handler,
		cache:   weak.NewMap[uint64, slog.Handler](),
	}
}

// Returns the base handler, which does not change after the cache is constructed.
func (hc *HandlerCache) Handler() slog.Handler {
	return hc.handler
}

func (hc *HandlerCache) get(id uint64) slog.Handler {
	v := hc.cache.Get(id)
	if v == nil {
		return nil
	}

	return *v
}

func (hc *HandlerCache) set(id uint64, h slog.Handler) {
	hc.cache.Set(id, &h)
}

type ResolveArgs struct {
	Level  slog.Level
	Record *slog.Record
}

// Used by a contextual handler to obtain the correct handler and associated
// cache data in a context-dependent way. The user of this package must
// implement this interface.
type ContextualResolver interface {
	// This method must choose the desired slog.Handler based on the provided
	// context, and then return a HandlerCache created from that slog.Handler by
	// calling NewHandlerCache. Note that the handler cache should be persistent
	// between calls to Resolve, as otherwise it defeats the object of the cache.
	//
	// args.Record is nil if this is being called as part of an Enable call, and
	// otherwise points to the record data about to be logged.
	Resolve(ctx context.Context, args ResolveArgs) *HandlerCache
}

// Convenience definition for defining ContextualResolver implementations.
type ContextualResolverFunc func(ctx context.Context, args ResolveArgs) *HandlerCache

// Implements ContextualResolver.
func (crf ContextualResolverFunc) Resolve(ctx context.Context, args ResolveArgs) *HandlerCache {
	return crf(ctx, args)
}

var _ ContextualResolver = ContextualResolverFunc(nil)

type contextualHandlerSet struct {
	resolver ContextualResolver
	nextID   uint64
}

func (s *contextualHandlerSet) getNextID() uint64 {
	return atomic.AddUint64(&s.nextID, 1)
}

type contextualHandler struct {
	s         *contextualHandlerSet
	parent    *contextualHandler
	attrs     []slog.Attr
	groupName string
	id        uint64
}

// Creates a new contextual handler. A contextual handler is a slog.Handler
// which can route calls to different slog.Handlers dynamically based on a
// context.Context, using an arbitrary routing predicate you provide.
//
// See ContextualResolver for details on the argument.
func NewContextualHandler(resolver ContextualResolver) slog.Handler {
	return &contextualHandler{
		s: &contextualHandlerSet{
			resolver: resolver,
		},
	}
}

var _ slog.Handler = &contextualHandler{}

func (ch *contextualHandler) Enabled(ctx context.Context, level slog.Level) bool {
	h := ch.resolveHandler(ctx, ResolveArgs{
		Level:  level,
		Record: nil,
	})
	return h.Enabled(ctx, level)
}

func (ch *contextualHandler) Handle(ctx context.Context, record slog.Record) error {
	h := ch.resolveHandler(ctx, ResolveArgs{
		Level:  record.Level,
		Record: &record,
	})
	return h.Handle(ctx, record)
}

func (ch *contextualHandler) resolveUsingCache(hc *HandlerCache) slog.Handler {
	if ch.parent == nil {
		return hc.Handler()
	}

	if h := hc.get(ch.id); h != nil {
		return h
	}

	h := ch.parent.resolveUsingCache(hc)

	if ch.attrs != nil {
		attrs := make([]slog.Attr, len(ch.attrs))
		copy(attrs, ch.attrs)

		h = h.WithAttrs(attrs)
	}

	if ch.groupName != "" {
		h = h.WithGroup(ch.groupName)
	}

	hc.set(ch.id, h)
	return h
}

func (ch *contextualHandler) resolveHandler(ctx context.Context, args ResolveArgs) slog.Handler {
	return ch.resolveUsingCache(ch.s.resolver.Resolve(ctx, args))
}

func (ch *contextualHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextualHandler{
		s:     ch.s,
		attrs: attrs,
		id:    ch.s.getNextID(),
	}
}

func (ch *contextualHandler) WithGroup(name string) slog.Handler {
	return &contextualHandler{
		s:         ch.s,
		groupName: name,
		id:        ch.s.getNextID(),
	}
}

// Simple Context Handler Store
type SimpleResolver struct {
	defaultCache *HandlerCache
}

// A simple resolver which obtains a HandlerCache by inspecting a context for
// the key set via the WithHandler() function. If none is set, uses the default
// handler given.
func NewSimpleResolver(defaultHandler slog.Handler) *SimpleResolver {
	return &SimpleResolver{
		defaultCache: NewHandlerCache(defaultHandler),
	}
}

var _ ContextualResolver = &SimpleResolver{}

type contextKey string

const key contextKey = "hc"

// Implements ContextualResolver.
func (sr *SimpleResolver) Resolve(ctx context.Context, args ResolveArgs) *HandlerCache {
	c, _ := ctx.Value(key).(*HandlerCache)
	if c == nil {
		return sr.defaultCache
	}
	return c
}

// Equivalent to creating a new handler using slog.Handler.WithArgs and then
// creating a new derived context using that handler using WithHandler.
func (sr *SimpleResolver) WithAttrs(ctx context.Context, args ...any) context.Context {
	return WithHandler(ctx, sr.Resolve(ctx, ResolveArgs{}).Handler().WithAttrs(argsToAttrSlice(args)))
}

// Equivalent to creating a new handler using slog.Handler.WithGroup and then
// creating a new derived context using that handler using WithHandler.
func (sr *SimpleResolver) WithGroup(ctx context.Context, name string) context.Context {
	return WithHandler(ctx, sr.Resolve(ctx, ResolveArgs{}).Handler().WithGroup(name))
}

// Creates a context derived from the given context but with the given
// HandlerCache associated with it.
func WithHandlerCache(ctx context.Context, hc *HandlerCache) context.Context {
	return context.WithValue(ctx, key, hc)
}

// Creates a context derived from the given context but with the given
// handler associated with it.
func WithHandler(ctx context.Context, handler slog.Handler) context.Context {
	return WithHandlerCache(ctx, NewHandlerCache(handler))
}

// Similar to SimpleResolver.WithAttrs, but does not need to be called on a
// SimpleResolver. However, it panics if there is no existing handler set on
// the context to derive from.
func WithAttrs(ctx context.Context, args ...any) context.Context {
	return WithHandler(ctx, cacheOrPanic(ctx).Handler().WithAttrs(argsToAttrSlice(args)))
}

// Similar to SimpleResolver.WithGroup, but does not need to be called on a
// SimpleResolver. However, it panics if there is no existing handler set on
// the context to derive from.
func WithGroup(ctx context.Context, name string) context.Context {
	c, _ := ctx.Value(key).(*HandlerCache)
	if c == nil {
		panic("")
	}

	return WithHandler(ctx, cacheOrPanic(ctx).Handler().WithGroup(name))
}

func cacheOrPanic(ctx context.Context) *HandlerCache {
	c, _ := ctx.Value(key).(*HandlerCache)
	if c == nil {
		panic("used slogextras.WithAttrs/WithGroup on context without existing slog handler")
	}

	return c
}

// slog/attr.go
func argsToAttrSlice(args []any) []slog.Attr {
	var (
		attr  slog.Attr
		attrs []slog.Attr
	)

	for len(args) > 0 {
		attr, args = argsToAttr(args)
		attrs = append(attrs, attr)

	}

	return attrs
}

const badKey = "!BADKEY"

// slog/record.go
func argsToAttr(args []any) (slog.Attr, []any) {
	switch x := args[0].(type) {
	case string:

		if len(args) == 1 {
			return slog.String(badKey, x), nil
		}

		return slog.Any(x, args[1]), args[2:]

	case slog.Attr:
		return x, args[1:]

	default:
		return slog.Any(badKey, x), args[1:]
	}
}

type routerHandler struct {
	m               sync.RWMutex
	enableFunc      func(ctx context.Context, level slog.Level) bool
	rules           []RouterRule
	derivedHandlers []slog.Handler
	attrs           []slog.Attr
	groupName       string
	parent          *routerHandler
}

// A routing rule which is processed in order.
//
// If MatchFunc() returns true, logging is passed to Handler. Examination of
// the rules then stops, unless Continue is set, in which the remaining rules
// continue to be evaluated and the message can potentially logged to multiple
// handlers.
type RouterRule struct {
	// The predicate function which will be called to determine whether this
	// rule matches.
	MatchFunc func(ctx context.Context, args ResolveArgs) bool
	// The handler to dispatch to if this rule matches.
	Handler slog.Handler
	// If not set, processing stops if this rule matches.
	Continue bool
}

// Creates a new router handler. This is a handler which will inspect the
// actual message being logged and can thereby choose to route it to one of a
// number of handlers.
//
// The rules are processed in order (see RouterRule). If enableFunc is non-nil,
// it will be used to provide the slog.Handler.Enabled function; otherwise
// Enabled will always return true (which is less efficient).
func NewRouterHandler(rules []RouterRule, enableFunc func(ctx context.Context, level slog.Level) bool) slog.Handler {
	rh := &routerHandler{
		rules:           rules,
		enableFunc:      enableFunc,
		derivedHandlers: make([]slog.Handler, len(rules)),
	}

	for i := range rules {
		rh.derivedHandlers[i] = rules[i].Handler
	}

	return rh
}

var _ slog.Handler = &routerHandler{}

func (rh *routerHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if rh.enableFunc == nil {
		return true
	}
	return rh.enableFunc(ctx, level)
}

func (rh *routerHandler) deriveHandler(i int) slog.Handler {
	var base slog.Handler
	if rh.parent != nil {
		base = rh.parent.deriveHandler(i)
	} else {
		base = rh.derivedHandlers[i]
	}

	if rh.attrs != nil {
		attrs := make([]slog.Attr, len(rh.attrs))
		copy(attrs, rh.attrs)
		base = base.WithAttrs(attrs)
	}

	if rh.groupName != "" {
		base = base.WithGroup(rh.groupName)
	}

	return base
}

func (rh *routerHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error

	for i := range rh.rules {
		r := &rh.rules[i]
		if r.MatchFunc(ctx, ResolveArgs{
			Level:  record.Level,
			Record: &record,
		}) {
			rh.m.RLock()
			subh := rh.derivedHandlers[i]
			if subh == nil {
				rh.m.RUnlock()
				rh.m.Lock()
				subh = rh.derivedHandlers[i]
				if subh == nil {
					subh = rh.deriveHandler(i)
					rh.derivedHandlers[i] = subh
				}
				rh.m.Unlock()
			}
			rh.m.RUnlock()
			err := subh.Handle(ctx, record)
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if !r.Continue {
				break
			}
		}
	}

	return firstErr
}

func (rh *routerHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &routerHandler{
		enableFunc:      rh.enableFunc,
		rules:           rh.rules,
		derivedHandlers: make([]slog.Handler, len(rh.rules)),
		parent:          rh,
		attrs:           attrs,
	}
}

func (rh *routerHandler) WithGroup(name string) slog.Handler {
	return &routerHandler{
		enableFunc:      rh.enableFunc,
		rules:           rh.rules,
		derivedHandlers: make([]slog.Handler, len(rh.rules)),
		parent:          rh,
		groupName:       name,
	}
}

type defaultHandler struct {
	m              sync.RWMutex
	parent         *defaultHandler
	attrs          []slog.Attr
	groupName      string
	expectedLogger *slog.Logger
	h              slog.Handler
}

var _ slog.Handler = &defaultHandler{}

// Creates a handler which always routes to the handler used by slog.Default().
func NewDefaultHandler() slog.Handler {
	return &defaultHandler{}
}

func (h *defaultHandler) update() slog.Handler {
	h.m.RLock()

	d := slog.Default()
	if h.h != nil && h.expectedLogger == d {
		res := h.h
		h.m.RUnlock()
		return res
	}

	h.m.RUnlock()
	h.m.Lock()
	defer h.m.Unlock()

	var newh slog.Handler
	if h.parent != nil {
		newh = h.parent.update()
	} else {
		newh = d.Handler()
	}

	if h.attrs != nil {
		attrs := make([]slog.Attr, len(h.attrs))
		copy(attrs, h.attrs)
		newh = newh.WithAttrs(attrs)
	}

	if h.groupName != "" {
		newh = newh.WithGroup(h.groupName)
	}

	h.h = newh
	h.expectedLogger = d
	return newh
}

func (h *defaultHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.update().Enabled(ctx, level)
}

func (h *defaultHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.update().Handle(ctx, record)
}

func (h *defaultHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &defaultHandler{
		parent: h,
		attrs:  attrs,
	}
}

func (h *defaultHandler) WithGroup(name string) slog.Handler {
	return &defaultHandler{
		parent:    h,
		groupName: name,
	}
}
