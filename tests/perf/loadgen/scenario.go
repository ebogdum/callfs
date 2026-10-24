package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"
)

type Env struct {
	Admin    *Client
	Users    []*Client
	Peers    []*Client
	Root     string
	Sizes    Sizes
	Features Features
	shared   sync.Map
}

type Sizes struct {
	Small int64
	Large int64
	Files int
}

type Features struct {
	Erasure     bool
	Replication bool
	S3          bool
	NATS        bool
	Cluster     bool
}

type Worker struct {
	Epoch  int64
	ID     int
	Iter   int
	Rand   *rand.Rand
	Client *Client
}

type Scenario struct {
	Name     string
	Group    string
	Needs    func(Features) bool
	Setup    func(ctx context.Context, env *Env) error
	Step     func(ctx context.Context, env *Env, w *Worker) error
	Teardown func(ctx context.Context, env *Env) error
}

var (
	registryMu sync.Mutex
	registry   = map[string]Scenario{}
)

func register(s Scenario) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[s.Name] = s
}

func scenarioNames() []string {
	registryMu.Lock()
	defer registryMu.Unlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func lookupScenario(name string) (Scenario, bool) {
	registryMu.Lock()
	defer registryMu.Unlock()
	s, ok := registry[name]
	return s, ok
}

func (e *Env) Path(scenario string, parts ...string) string {
	p := e.Root + "/" + scenario
	for _, part := range parts {
		p += "/" + part
	}
	return p
}

func (e *Env) Put(key string, value any) {
	e.shared.Store(key, value)
}

func (e *Env) Get(key string) (any, bool) {
	return e.shared.Load(key)
}

func (w *Worker) Pick(n int) int {
	if 0 >= n {
		return 0
	}
	return w.Rand.IntN(n)
}

func (w *Worker) Name(prefix string) string {
	return fmt.Sprintf("%s-e%d-w%d-i%d", prefix, w.Epoch, w.ID, w.Iter)
}

func (e *Env) User(w *Worker) *Client {
	if 0 == len(e.Users) {
		return e.Admin
	}
	return e.Users[w.ID%len(e.Users)]
}
