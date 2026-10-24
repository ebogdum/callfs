package main

import (
	"context"
	"fmt"
	"net/http"
)

const filesGroup = "files"

type filesSeedSet struct {
	Paths []string
	Sizes []int64
}

func filesAPI(path string) string {
	return "/v1/files" + path
}

func filesPutPayload(ctx context.Context, c *Client, op, path string, size int64, seed int, expect []int, header map[string]string) (Response, error) {
	return c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodPut,
		Path:   filesAPI(path),
		Body:   Payload(size, seed),
		Size:   size,
		Header: header,
		Expect: expect,
	})
}

func filesSeed(ctx context.Context, env *Env, scenario string, count int, size int64, largeCount int) error {
	set := &filesSeedSet{}
	for i := 0; i < count+largeCount; i++ {
		fileSize := size
		name := fmt.Sprintf("seed-%d", i)
		if i >= count {
			fileSize = env.Sizes.Large
			name = fmt.Sprintf("seed-large-%d", i-count)
		}
		path := env.Path(scenario, "seed", name)
		if _, err := filesPutPayload(ctx, env.Admin, "files.seed", path, fileSize, i, []int{http.StatusCreated, http.StatusOK}, nil); nil != err {
			return fmt.Errorf("seed %s: %w", path, err)
		}
		set.Paths = append(set.Paths, path)
		set.Sizes = append(set.Sizes, fileSize)
	}
	env.Put(scenario+".seed", set)
	return nil
}

func filesSeedSetOf(env *Env, scenario string) (*filesSeedSet, error) {
	value, ok := env.Get(scenario + ".seed")
	if !ok {
		return nil, fmt.Errorf("scenario %s has no seed data", scenario)
	}
	set, ok := value.(*filesSeedSet)
	if !ok || nil == set || 0 == len(set.Paths) {
		return nil, fmt.Errorf("scenario %s has invalid seed data", scenario)
	}
	return set, nil
}

func filesTeardown(scenario string) func(ctx context.Context, env *Env) error {
	return func(ctx context.Context, env *Env) error {
		_, err := env.Admin.Do(ctx, Request{
			Op:     "files.teardown",
			Method: http.MethodDelete,
			Path:   filesAPI(env.Path(scenario)) + "/?recursive=true",
			Expect: []int{http.StatusOK, http.StatusNoContent, http.StatusNotFound},
		})
		if nil != err {
			return fmt.Errorf("teardown %s: %w", scenario, err)
		}
		return nil
	}
}

func filesSmallSize(env *Env) int64 {
	if 1 > env.Sizes.Small {
		return 1
	}
	return env.Sizes.Small
}

func filesETag(resp Response) (string, error) {
	etag := resp.Header.Get("ETag")
	if "" == etag {
		return "", fmt.Errorf("response has no ETag header")
	}
	return etag, nil
}

func init() {
	register(Scenario{
		Name:     "files-put-small",
		Group:    filesGroup,
		Teardown: filesTeardown("files-put-small"),
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			size := filesSmallSize(env)
			putPath := env.Path("files-put-small", fmt.Sprintf("w%d", w.ID), w.Name("put"))
			if _, err := filesPutPayload(ctx, w.Client, "files.put_small", putPath, size, w.Iter, []int{http.StatusCreated}, nil); nil != err {
				return fmt.Errorf("put small: %w", err)
			}
			postPath := env.Path("files-put-small", fmt.Sprintf("w%d", w.ID), w.Name("post"))
			if _, err := w.Client.Do(ctx, Request{
				Op:     "files.post_small",
				Method: http.MethodPost,
				Path:   filesAPI(postPath),
				Body:   Payload(size, w.Iter+1),
				Size:   size,
				Expect: []int{http.StatusCreated},
			}); nil != err {
				return fmt.Errorf("post small: %w", err)
			}
			return nil
		},
	})

	register(Scenario{
		Name:     "files-put-large",
		Group:    filesGroup,
		Teardown: filesTeardown("files-put-large"),
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			path := env.Path("files-put-large", fmt.Sprintf("w%d", w.ID), w.Name("large"))
			if _, err := filesPutPayload(ctx, w.Client, "files.put_large", path, env.Sizes.Large, w.Iter, []int{http.StatusCreated}, nil); nil != err {
				return fmt.Errorf("put large: %w", err)
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:     "files.put_large_cleanup",
				Method: http.MethodDelete,
				Path:   filesAPI(path),
				Expect: []int{http.StatusNoContent, http.StatusOK},
			}); nil != err {
				return fmt.Errorf("delete large: %w", err)
			}
			return nil
		},
	})

	register(Scenario{
		Name:  "files-overwrite",
		Group: filesGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return filesSeed(ctx, env, "files-overwrite", env.Sizes.Files, filesSmallSize(env), 0)
		},
		Teardown: filesTeardown("files-overwrite"),
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			set, err := filesSeedSetOf(env, "files-overwrite")
			if nil != err {
				return err
			}
			idx := w.Pick(len(set.Paths))
			if _, err := filesPutPayload(ctx, env.Admin, "files.overwrite", set.Paths[idx], set.Sizes[idx], w.Iter+w.ID, []int{http.StatusOK}, nil); nil != err {
				return fmt.Errorf("overwrite: %w", err)
			}
			return nil
		},
	})

	register(Scenario{
		Name:  "files-get",
		Group: filesGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return filesSeed(ctx, env, "files-get", env.Sizes.Files, filesSmallSize(env), 2)
		},
		Teardown: filesTeardown("files-get"),
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			set, err := filesSeedSetOf(env, "files-get")
			if nil != err {
				return err
			}
			idx := w.Pick(len(set.Paths))
			op := "files.get_small"
			if set.Sizes[idx] == env.Sizes.Large && env.Sizes.Large != filesSmallSize(env) {
				op = "files.get_large"
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:      op,
				Method:  http.MethodGet,
				Path:    filesAPI(set.Paths[idx]),
				Discard: true,
				Expect:  []int{http.StatusOK},
			}); nil != err {
				return fmt.Errorf("get: %w", err)
			}
			return nil
		},
	})

	register(Scenario{
		Name:  "files-get-range",
		Group: filesGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return filesSeed(ctx, env, "files-get-range", env.Sizes.Files, filesSmallSize(env), 2)
		},
		Teardown: filesTeardown("files-get-range"),
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			set, err := filesSeedSetOf(env, "files-get-range")
			if nil != err {
				return err
			}
			idx := w.Pick(len(set.Paths))
			size := set.Sizes[idx]
			span := int64(64 << 10)
			if span > size {
				span = size
			}
			start := int64(0)
			if size > span {
				start = w.Rand.Int64N(size - span + 1)
			}
			end := start + span - 1
			op := "files.get_range"
			header := map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", start, end)}
			switch w.Pick(4) {
			case 0:
				op = "files.get_range_suffix"
				suffix := span
				header = map[string]string{"Range": fmt.Sprintf("bytes=-%d", suffix)}
			case 1:
				op = "files.get_range_open"
				header = map[string]string{"Range": fmt.Sprintf("bytes=%d-", start)}
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:      op,
				Method:  http.MethodGet,
				Path:    filesAPI(set.Paths[idx]),
				Header:  header,
				Discard: true,
				Expect:  []int{http.StatusPartialContent},
			}); nil != err {
				return fmt.Errorf("get range: %w", err)
			}
			return nil
		},
	})

	register(Scenario{
		Name:  "files-head-stat",
		Group: filesGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return filesSeed(ctx, env, "files-head-stat", env.Sizes.Files, filesSmallSize(env), 0)
		},
		Teardown: filesTeardown("files-head-stat"),
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			set, err := filesSeedSetOf(env, "files-head-stat")
			if nil != err {
				return err
			}
			path := set.Paths[w.Pick(len(set.Paths))]
			if _, err := w.Client.Do(ctx, Request{
				Op:     "files.head",
				Method: http.MethodHead,
				Path:   filesAPI(path),
				Expect: []int{http.StatusOK},
			}); nil != err {
				return fmt.Errorf("head: %w", err)
			}
			var stat struct {
				Type string `json:"type"`
				Size int64  `json:"size"`
				ETag string `json:"etag"`
			}
			if err := w.Client.JSON(ctx, Request{
				Op:     "files.stat",
				Method: http.MethodGet,
				Path:   "/v1/stat" + path,
				Expect: []int{http.StatusOK},
			}, &stat); nil != err {
				return fmt.Errorf("stat file: %w", err)
			}
			if err := w.Client.JSON(ctx, Request{
				Op:     "files.stat_dir",
				Method: http.MethodGet,
				Path:   "/v1/stat" + env.Path("files-head-stat", "seed"),
				Expect: []int{http.StatusOK},
			}, &stat); nil != err {
				return fmt.Errorf("stat dir: %w", err)
			}
			missing := env.Path("files-head-stat", "seed", w.Name("missing"))
			if _, err := w.Client.Do(ctx, Request{
				Op:     "files.head_missing",
				Method: http.MethodHead,
				Path:   filesAPI(missing),
				Expect: []int{http.StatusNotFound},
			}); nil != err {
				return fmt.Errorf("head missing: %w", err)
			}
			return nil
		},
	})

	register(Scenario{
		Name:  "files-conditional",
		Group: filesGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return filesSeed(ctx, env, "files-conditional", env.Sizes.Files, filesSmallSize(env), 0)
		},
		Teardown: filesTeardown("files-conditional"),
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			size := filesSmallSize(env)
			path := env.Path("files-conditional", fmt.Sprintf("w%d", w.ID), w.Name("cond"))
			created, err := filesPutPayload(ctx, w.Client, "files.cond_create", path, size, w.Iter, []int{http.StatusCreated}, map[string]string{"If-None-Match": "*"})
			if nil != err {
				return fmt.Errorf("cond create: %w", err)
			}
			if _, err := filesPutPayload(ctx, w.Client, "files.cond_put_exists", path, size, w.Iter, []int{http.StatusPreconditionFailed}, map[string]string{"If-None-Match": "*"}); nil != err {
				return fmt.Errorf("cond put exists: %w", err)
			}
			etag, err := filesETag(created)
			if nil != err {
				return fmt.Errorf("cond create: %w", err)
			}
			got, err := w.Client.Do(ctx, Request{
				Op:      "files.cond_get",
				Method:  http.MethodGet,
				Path:    filesAPI(path),
				Discard: true,
				Expect:  []int{http.StatusOK},
			})
			if nil != err {
				return fmt.Errorf("cond get: %w", err)
			}
			if etag, err = filesETag(got); nil != err {
				return fmt.Errorf("cond get: %w", err)
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:     "files.cond_get_304",
				Method: http.MethodGet,
				Path:   filesAPI(path),
				Header: map[string]string{"If-None-Match": etag},
				Expect: []int{http.StatusNotModified},
			}); nil != err {
				return fmt.Errorf("cond get 304: %w", err)
			}
			_, err = filesPutPayload(ctx, w.Client, "files.cond_put_match", path, size, w.Iter+1, []int{http.StatusOK}, map[string]string{"If-Match": etag})
			if nil != err {
				return fmt.Errorf("cond put match: %w", err)
			}
			if _, err := filesPutPayload(ctx, w.Client, "files.cond_put_stale", path, size, w.Iter+2, []int{http.StatusPreconditionFailed}, map[string]string{"If-Match": etag}); nil != err {
				return fmt.Errorf("cond put stale: %w", err)
			}
			if _, err := filesPutPayload(ctx, w.Client, "files.cond_put_bogus", path, size, w.Iter+3, []int{http.StatusPreconditionFailed}, map[string]string{"If-Match": `"stale-etag"`}); nil != err {
				return fmt.Errorf("cond put bogus: %w", err)
			}
			return nil
		},
	})

	register(Scenario{
		Name:     "files-mkdir",
		Group:    filesGroup,
		Teardown: filesTeardown("files-mkdir"),
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			base := env.Path("files-mkdir", fmt.Sprintf("w%d", w.ID), w.Name("tree"))
			if _, err := w.Client.Do(ctx, Request{
				Op:     "files.mkdir",
				Method: http.MethodPost,
				Path:   filesAPI(base) + "/",
				Expect: []int{http.StatusCreated},
			}); nil != err {
				return fmt.Errorf("mkdir: %w", err)
			}
			nested := base
			for _, level := range []string{"a", "b", "c", "d"} {
				nested += "/" + level
				if _, err := w.Client.Do(ctx, Request{
					Op:     "files.mkdir_nested",
					Method: http.MethodPost,
					Path:   filesAPI(nested) + "/",
					Expect: []int{http.StatusCreated},
				}); nil != err {
					return fmt.Errorf("mkdir nested: %w", err)
				}
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:     "files.mkdir_exists",
				Method: http.MethodPost,
				Path:   filesAPI(base) + "/",
				Expect: []int{http.StatusOK},
			}); nil != err {
				return fmt.Errorf("mkdir exists: %w", err)
			}
			return nil
		},
	})
}
