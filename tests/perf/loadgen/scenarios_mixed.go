package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

const (
	contentionScenario = "contention-same-file"
	hotDirScenario     = "contention-hot-dir"
	mixedScenario      = "mixed"
	contentionFiles    = 4
	mixedKey           = "mixed.parts"
)

var mixedWeights = []struct {
	name   string
	weight int
}{
	{"files-get", 20},
	{"files-get-range", 10},
	{"files-head-stat", 10},
	{"files-put-small", 12},
	{"files-overwrite", 6},
	{"files-conditional", 4},
	{"tree-list", 10},
	{"tree-search", 4},
	{"tree-rename", 4},
	{"tree-delete-recursive", 2},
	{"links-generate", 3},
	{"links-download", 3},
	{"trash-cycle", 2},
	{"uploads-multipart", 2},
	{"admin-read", 4},
	{"account", 4},
	{"erasure-write-read", 4},
	{"cluster-cross-instance", 4},
}

func init() {
	register(Scenario{
		Name:  contentionScenario,
		Group: "contention",
		Setup: func(ctx context.Context, env *Env) error {
			for i := 0; i < contentionFiles; i++ {
				if _, err := env.Admin.Do(ctx, Request{Op: "setup.put", Method: http.MethodPut, Path: "/v1/files" + env.Path(contentionScenario, fmt.Sprintf("hot-%d.bin", i)), Body: Payload(env.Sizes.Small, i), Size: env.Sizes.Small}); nil != err {
					return err
				}
			}
			return nil
		},
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			path := "/v1/files" + env.Path(contentionScenario, fmt.Sprintf("hot-%d.bin", w.Pick(contentionFiles)))
			if 0 == w.Iter%3 {
				_, err := w.Client.Do(ctx, Request{Op: "contention.get", Method: http.MethodGet, Path: path, Discard: true})
				return err
			}
			_, err := env.Admin.Do(ctx, Request{Op: "contention.put", Method: http.MethodPut, Path: path, Body: Payload(env.Sizes.Small, w.Iter), Size: env.Sizes.Small, Expect: []int{http.StatusOK, http.StatusCreated, http.StatusNoContent, http.StatusConflict, http.StatusLocked, http.StatusServiceUnavailable}})
			return err
		},
		Teardown: func(ctx context.Context, env *Env) error {
			return deleteTree(ctx, env, contentionScenario)
		},
	})
	register(Scenario{
		Name:  hotDirScenario,
		Group: "contention",
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			_, err := w.Client.Do(ctx, Request{Op: "hotdir.create", Method: http.MethodPut, Path: "/v1/files" + env.Path(hotDirScenario, w.Name("f")+".bin"), Body: Payload(env.Sizes.Small/4, w.Iter), Size: env.Sizes.Small / 4})
			return err
		},
		Teardown: func(ctx context.Context, env *Env) error {
			return deleteTree(ctx, env, hotDirScenario)
		},
	})
	register(Scenario{
		Name:     mixedScenario,
		Group:    "mixed",
		Setup:    setupMixed,
		Step:     stepMixed,
		Teardown: teardownMixed,
	})
}

func deleteTree(ctx context.Context, env *Env, scenario string) error {
	_, err := env.Admin.Do(ctx, Request{Op: "teardown.delete", Method: http.MethodDelete, Path: "/v1/files" + env.Path(scenario) + "?recursive=true", Expect: []int{http.StatusOK, http.StatusNoContent, http.StatusAccepted, http.StatusNotFound}})
	return err
}

type mixedPart struct {
	scenario Scenario
	weight   int
}

func setupMixed(ctx context.Context, env *Env) error {
	parts := []mixedPart{}
	total := 0
	var errs []error
	for _, entry := range mixedWeights {
		s, ok := lookupScenario(entry.name)
		if !ok || (nil != s.Needs && !s.Needs(env.Features)) {
			continue
		}
		if nil != s.Setup {
			if err := s.Setup(ctx, env); nil != err {
				errs = append(errs, fmt.Errorf("%s: %w", entry.name, err))
				continue
			}
		}
		parts = append(parts, mixedPart{scenario: s, weight: entry.weight})
		total += entry.weight
	}
	if 0 == total {
		return errors.Join(append(errs, errors.New("no mixed parts available"))...)
	}
	env.Put(mixedKey, parts)
	return nil
}

func stepMixed(ctx context.Context, env *Env, w *Worker) error {
	value, ok := env.Get(mixedKey)
	if !ok {
		return errors.New("mixed parts missing")
	}
	parts, _ := value.([]mixedPart)
	total := 0
	for _, p := range parts {
		total += p.weight
	}
	pick := w.Pick(total)
	for _, p := range parts {
		if pick < p.weight {
			return p.scenario.Step(ctx, env, w)
		}
		pick -= p.weight
	}
	return nil
}

func teardownMixed(ctx context.Context, env *Env) error {
	value, ok := env.Get(mixedKey)
	if !ok {
		return nil
	}
	parts, _ := value.([]mixedPart)
	var errs []error
	for _, p := range parts {
		if nil != p.scenario.Teardown {
			if err := p.scenario.Teardown(ctx, env); nil != err {
				errs = append(errs, fmt.Errorf("%s: %w", p.scenario.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}
