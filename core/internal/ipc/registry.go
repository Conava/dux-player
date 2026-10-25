package ipc

import (
	"context"
	"fmt"
	"sort"
)

type ParamSpec struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Desc     string `json:"desc,omitempty"`
}

type MethodSpec struct {
	Name   string      `json:"name"`
	Group  string      `json:"group"`
	Desc   string      `json:"desc,omitempty"`
	Params []ParamSpec `json:"params,omitempty"`
	// Read marks a method that changes nothing. Reads run in parallel;
	// everything else runs in the order the client sent it.
	Read bool `json:"-"`
}

func Req(name, desc string) ParamSpec { return ParamSpec{Name: name, Required: true, Desc: desc} }
func Opt(name, desc string) ParamSpec { return ParamSpec{Name: name, Desc: desc} }

// Handler answers one request. It must write exactly one reply.
type Handler func(ctx context.Context, w *ConnWriter, req Request)

// Registry is the method catalog and routing table; each feature adds its
// own. The CLI builds it too for `dplayer ipc list` and completion, so
// registering must not touch the dependencies a handler uses.
type Registry struct {
	specs       []MethodSpec
	byName      map[string]int
	handlers    map[string]Handler
	topics      []string
	onSubscribe []func()
	greet       []func(send func(topic string, data any))
}

func NewRegistry() *Registry {
	return &Registry{byName: map[string]int{}, handlers: map[string]Handler{}}
}

// Group registers methods that share one handler, which switches on
// req.Method. Every spec's Group is set to group. A name registered twice
// is a programming error and panics at startup.
func (r *Registry) Group(group string, h Handler, specs ...MethodSpec) {
	for _, s := range specs {
		if _, dup := r.handlers[s.Name]; dup {
			panic(fmt.Sprintf("ipc method %s registered twice", s.Name))
		}
		s.Group = group
		r.byName[s.Name] = len(r.specs)
		r.specs = append(r.specs, s)
		r.handlers[s.Name] = h
	}
}

// Topics adds event topics to the default subscription.
func (r *Registry) Topics(topics ...string) { r.topics = append(r.topics, topics...) }

// OnSubscribe runs f whenever subscriptions change (subscribe, unsubscribe,
// a client going away), so a feature can publish its current state to a new
// subscriber or stop work nobody listens to any more.
func (r *Registry) OnSubscribe(f func()) { r.onSubscribe = append(r.onSubscribe, f) }

// Greet runs f after every subscribe with a send that reaches only the new
// subscriber (and only its topics), for state other clients already have.
func (r *Registry) Greet(f func(send func(topic string, data any))) { r.greet = append(r.greet, f) }

func (r *Registry) Specs() []MethodSpec { return r.specs }

func (r *Registry) Names() []string {
	names := make([]string, len(r.specs))
	for i, m := range r.specs {
		names[i] = m.Name
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Find(name string) (MethodSpec, bool) {
	i, ok := r.byName[name]
	if !ok {
		return MethodSpec{}, false
	}
	return r.specs[i], true
}

// groups are the handshake capabilities: one per registered group.
func (r *Registry) groups() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range r.specs {
		if !seen[m.Group] {
			seen[m.Group] = true
			out = append(out, m.Group)
		}
	}
	return out
}

func (r *Registry) isRead(method string) bool {
	m, ok := r.Find(method)
	return ok && m.Read
}

func (r *Registry) route(ctx context.Context, w *ConnWriter, req Request) {
	h, ok := r.handlers[req.Method]
	if !ok {
		w.RespondError(req.ID, "unknown method: "+req.Method)
		return
	}
	h(ctx, w, req)
}
