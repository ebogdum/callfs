package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	treeListName    = "tree-list"
	treeObjectsName = "tree-objects"
	treeSearchName  = "tree-search"
	treeRenameName  = "tree-rename"
	treeDeleteName  = "tree-delete-recursive"
	treeBatchName   = "tree-batch"
	treeSeedWorkers = 8
	treeJobPollWait = 25 * time.Millisecond
	treeJobDeadline = 60 * time.Second
)

type treePage struct {
	NextCursor *string `json:"next_cursor"`
}

type treeObjectList struct {
	Objects []struct {
		Path string `json:"path"`
	} `json:"objects"`
	Truncated      bool   `json:"truncated"`
	NextStartAfter string `json:"next_start_after"`
}

type treeSearchResult struct {
	Items []struct {
		Path string `json:"path"`
	} `json:"items"`
	NextCursor string `json:"next_cursor"`
}

type treeBatchResponse struct {
	Results []struct {
		Index   int    `json:"index"`
		Status  string `json:"status"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Path    string `json:"path"`
	} `json:"results"`
}

type treeJobSubmit struct {
	JobID string `json:"job_id"`
}

type treeJob struct {
	State string  `json:"state"`
	Error *string `json:"error"`
}

func treeFile(p string) string {
	return "/v1/files" + p
}

func treeQuery(base string, values url.Values) string {
	return base + "?" + values.Encode()
}

func treeCount(env *Env) int {
	return max(env.Sizes.Files, 1)
}

func treePut(ctx context.Context, c *Client, op, p string, size int64, seed int) error {
	size = max(size, 1)
	_, err := c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodPut,
		Path:   treeFile(p),
		Body:   Payload(size, seed),
		Size:   size,
		Expect: []int{http.StatusOK, http.StatusCreated},
	})
	if nil != err {
		return fmt.Errorf("put %s: %w", p, err)
	}
	return nil
}

func treeMkdir(ctx context.Context, c *Client, op, p string) error {
	_, err := c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodPost,
		Path:   treeFile(p) + "/",
		Expect: []int{http.StatusOK, http.StatusCreated},
	})
	if nil != err {
		return fmt.Errorf("mkdir %s: %w", p, err)
	}
	return nil
}

func treePatch(ctx context.Context, c *Client, op, p, query string, body any, expect ...int) error {
	target := treeFile(p)
	if "" != query {
		target += "?" + query
	}
	if 0 == len(expect) {
		expect = []int{http.StatusOK}
	}
	_, err := c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodPatch,
		Path:   target,
		JSON:   body,
		Expect: expect,
	})
	if nil != err {
		return fmt.Errorf("patch %s: %w", p, err)
	}
	return nil
}

func treeDeleteTree(ctx context.Context, c *Client, op, p string) error {
	_, err := c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodDelete,
		Path:   treeFile(p) + "?recursive=true",
		Expect: []int{http.StatusOK, http.StatusNoContent, http.StatusNotFound},
	})
	if nil != err {
		return fmt.Errorf("delete tree %s: %w", p, err)
	}
	return nil
}

func treeSeed(ctx context.Context, c *Client, op string, paths []string, size int64) error {
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for i := 0; i < treeSeedWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if err := treePut(ctx, c, op, paths[index], size, index); nil != err {
					mu.Lock()
					if nil == first {
						first = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	for index := range paths {
		if nil != ctx.Err() {
			break
		}
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	if nil != first {
		return first
	}
	return ctx.Err()
}

func treeTeardown(name string) func(ctx context.Context, env *Env) error {
	return func(ctx context.Context, env *Env) error {
		return treeDeleteTree(ctx, env.Admin, "teardown.delete_tree", env.Path(name))
	}
}

func treeSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func init() {
	register(Scenario{
		Name:     treeListName,
		Group:    "tree",
		Setup:    treeListSetup,
		Step:     treeListStep,
		Teardown: treeTeardown(treeListName),
	})
	register(Scenario{
		Name:     treeObjectsName,
		Group:    "tree",
		Setup:    treeObjectsSetup,
		Step:     treeObjectsStep,
		Teardown: treeTeardown(treeObjectsName),
	})
	register(Scenario{
		Name:     treeSearchName,
		Group:    "tree",
		Setup:    treeSearchSetup,
		Step:     treeSearchStep,
		Teardown: treeTeardown(treeSearchName),
	})
	register(Scenario{
		Name:     treeRenameName,
		Group:    "tree",
		Step:     treeRenameStep,
		Teardown: treeTeardown(treeRenameName),
	})
	register(Scenario{
		Name:     treeDeleteName,
		Group:    "tree",
		Step:     treeDeleteStep,
		Teardown: treeTeardown(treeDeleteName),
	})
	register(Scenario{
		Name:     treeBatchName,
		Group:    "tree",
		Step:     treeBatchStep,
		Teardown: treeTeardown(treeBatchName),
	})
}

func treeListSetup(ctx context.Context, env *Env) error {
	wide := env.Path(treeListName, "wide")
	paths := make([]string, 0, treeCount(env)+64)
	for i := 0; i < treeCount(env); i++ {
		paths = append(paths, fmt.Sprintf("%s/f-%05d", wide, i))
	}
	for i := 0; i < 5; i++ {
		paths = append(paths, fmt.Sprintf("%s/.hidden-%d", wide, i))
	}
	for d := 0; d < 4; d++ {
		for s := 0; s < 3; s++ {
			for f := 0; f < 3; f++ {
				paths = append(paths, env.Path(treeListName, "nested", fmt.Sprintf("d%d", d), fmt.Sprintf("s%d", s), fmt.Sprintf("n-%d", f)))
			}
		}
	}
	for d := 0; d < 3; d++ {
		if err := treeMkdir(ctx, env.Admin, "seed.mkdir", env.Path(treeListName, "wide", fmt.Sprintf("sub-%d", d))); nil != err {
			return err
		}
	}
	if err := treeSeed(ctx, env.Admin, "seed.put", paths, env.Sizes.Small); nil != err {
		return err
	}
	env.Put(treeListName+".wide", wide)
	return nil
}

func treeListQuery(w *Worker, extra url.Values) url.Values {
	values := url.Values{}
	values.Set("limit", strconv.Itoa(25+w.Pick(76)))
	values.Set("sort", []string{"name", "size", "mtime", "type"}[w.Pick(4)])
	values.Set("order", []string{"asc", "desc"}[w.Pick(2)])
	for key, list := range extra {
		values[key] = list
	}
	return values
}

func treeListStep(ctx context.Context, env *Env, w *Worker) error {
	wide := env.Path(treeListName, "wide")
	root := env.Path(treeListName)
	switch w.Pick(6) {
	case 0:
		_, err := w.Client.Do(ctx, Request{Op: "directories.list", Method: http.MethodGet, Path: "/v1/directories" + wide})
		return err
	case 1:
		values := url.Values{}
		values.Set("recursive", "true")
		values.Set("max_depth", strconv.Itoa(2+w.Pick(4)))
		_, err := w.Client.Do(ctx, Request{Op: "directories.list_recursive", Method: http.MethodGet, Path: treeQuery("/v1/directories"+root, values)})
		return err
	case 2:
		var out treePage
		return w.Client.JSON(ctx, Request{Op: "directories.page", Method: http.MethodGet, Path: treeQuery("/v1/directories"+wide, treeListQuery(w, nil))}, &out)
	case 3:
		cursor := ""
		for page := 0; 5 > page; page++ {
			values := treeListQuery(w, url.Values{"limit": {"50"}, "sort": {"name"}, "order": {"asc"}})
			op := "directories.page_first"
			if "" != cursor {
				values.Set("cursor", cursor)
				op = "directories.page_next"
			}
			var out treePage
			if err := w.Client.JSON(ctx, Request{Op: op, Method: http.MethodGet, Path: treeQuery("/v1/directories"+wide, values)}, &out); nil != err {
				return err
			}
			if nil == out.NextCursor {
				return nil
			}
			cursor = *out.NextCursor
		}
		return nil
	case 4:
		values := treeListQuery(w, url.Values{"q": {"f-000"}, "type": {"file"}, "include_hidden": {"false"}})
		var out treePage
		return w.Client.JSON(ctx, Request{Op: "directories.page_filter", Method: http.MethodGet, Path: treeQuery("/v1/directories"+wide, values)}, &out)
	default:
		values := treeListQuery(w, url.Values{"type": {"directory"}})
		var out treePage
		return w.Client.JSON(ctx, Request{Op: "directories.page_dirs", Method: http.MethodGet, Path: treeQuery("/v1/directories"+env.Path(treeListName, "nested"), values)}, &out)
	}
}

func treeObjectsSetup(ctx context.Context, env *Env) error {
	paths := make([]string, 0, treeCount(env))
	for i := 0; i < treeCount(env); i++ {
		paths = append(paths, env.Path(treeObjectsName, "seed", fmt.Sprintf("f-%05d", i)))
	}
	return treeSeed(ctx, env.Admin, "seed.put", paths, env.Sizes.Small)
}

func treeObjectsStep(ctx context.Context, env *Env, w *Worker) error {
	c := w.Client
	seeded := url.Values{}
	seeded.Set("prefix", env.Path(treeObjectsName, "seed")+"/")
	seeded.Set("limit", "100")
	seeded.Set("start_after", env.Path(treeObjectsName, "seed", fmt.Sprintf("f-%05d", w.Pick(treeCount(env)))))
	var seededOut treeObjectList
	if err := c.JSON(ctx, Request{Op: "objects.list_seeded", Method: http.MethodGet, Path: treeQuery("/v1/objects", seeded)}, &seededOut); nil != err {
		return err
	}

	dir := env.Path(treeObjectsName, w.Name("p"))
	files := min(10, treeCount(env))
	for i := 0; i < files; i++ {
		if err := treePut(ctx, c, "objects.seed_put", fmt.Sprintf("%s/f-%03d", dir, i), env.Sizes.Small, i); nil != err {
			return err
		}
	}

	startAfter := ""
	for page := 0; 3 > page; page++ {
		values := url.Values{}
		values.Set("prefix", dir+"/")
		values.Set("limit", "4")
		op := "objects.list"
		if "" != startAfter {
			values.Set("start_after", startAfter)
			op = "objects.list_next"
		}
		var out treeObjectList
		if err := c.JSON(ctx, Request{Op: op, Method: http.MethodGet, Path: treeQuery("/v1/objects", values)}, &out); nil != err {
			return err
		}
		if !out.Truncated || "" == out.NextStartAfter {
			break
		}
		startAfter = out.NextStartAfter
	}

	count := url.Values{}
	count.Set("prefix", dir+"/")
	count.Set("count_only", "true")
	if _, err := c.Do(ctx, Request{Op: "objects.count", Method: http.MethodGet, Path: treeQuery("/v1/objects", count)}); nil != err {
		return err
	}

	del := url.Values{}
	del.Set("prefix", dir)
	del.Set("boundary", "true")
	del.Set("dry_run", "true")
	if _, err := c.Do(ctx, Request{Op: "objects.delete_dry_run", Method: http.MethodDelete, Path: treeQuery("/v1/objects", del)}); nil != err {
		return err
	}
	del.Del("dry_run")
	if _, err := c.Do(ctx, Request{Op: "objects.delete_prefix", Method: http.MethodDelete, Path: treeQuery("/v1/objects", del), Expect: []int{http.StatusOK}}); nil != err {
		return err
	}
	return nil
}

func treeSearchSetup(ctx context.Context, env *Env) error {
	kinds := []string{"report", "image", "doc", "log"}
	paths := make([]string, 0, treeCount(env))
	for i := 0; i < treeCount(env); i++ {
		group := fmt.Sprintf("g%d", i%5)
		name := fmt.Sprintf("%s-%05d.txt", kinds[i%len(kinds)], i)
		if 0 == i%3 {
			paths = append(paths, env.Path(treeSearchName, group, "sub", name))
			continue
		}
		paths = append(paths, env.Path(treeSearchName, group, name))
	}
	for i, p := range paths {
		size := env.Sizes.Small
		if 1 == i%2 {
			size = max(1, env.Sizes.Small/2)
		}
		if err := treePut(ctx, env.Admin, "seed.put", p, size, i); nil != err {
			return err
		}
	}
	return nil
}

func treeSearchStep(ctx context.Context, env *Env, w *Worker) error {
	kinds := []string{"report", "image", "doc", "log"}
	values := url.Values{}
	values.Set("path", env.Path(treeSearchName))
	op := ""
	switch w.Pick(9) {
	case 0:
		op = "search.name"
		values.Set("q", "report")
	case 1:
		op = "search.glob"
		values.Set("q", "doc-001*")
	case 2:
		op = "search.name_limit"
		values.Set("q", kinds[w.Pick(len(kinds))])
		values.Set("type", "file")
		values.Set("limit", "500")
	case 3:
		op = "search.dirs"
		values.Set("type", "directory")
	case 4:
		op = "search.size_range"
		values.Set("min_size", strconv.FormatInt(env.Sizes.Small, 10))
		values.Set("max_size", strconv.FormatInt(env.Sizes.Small, 10))
	case 5:
		op = "search.modified"
		values.Set("q", "log")
		values.Set("modified_after", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
	case 6:
		op = "search.facets"
		values.Set("facets", "true")
		values.Set("limit", "10")
	case 7:
		op = "search.storage"
		storage := "localfs"
		if env.Features.S3 {
			storage = "s3"
		}
		values.Set("storage", storage)
		values.Set("erasure", "false")
		values.Set("q", "image")
	default:
		cursor := ""
		for page := 0; 4 > page; page++ {
			paged := url.Values{}
			paged.Set("path", env.Path(treeSearchName))
			paged.Set("limit", "20")
			pageOp := "search.page_first"
			if "" != cursor {
				paged.Set("cursor", cursor)
				pageOp = "search.page_next"
			}
			var out treeSearchResult
			if err := w.Client.JSON(ctx, Request{Op: pageOp, Method: http.MethodGet, Path: treeQuery("/v1/search", paged)}, &out); nil != err {
				return err
			}
			if "" == out.NextCursor {
				return nil
			}
			cursor = out.NextCursor
		}
		return nil
	}
	var out treeSearchResult
	return w.Client.JSON(ctx, Request{Op: op, Method: http.MethodGet, Path: treeQuery("/v1/search", values)}, &out)
}

func treeRenameStep(ctx context.Context, env *Env, w *Worker) error {
	c := w.Client
	size := env.Sizes.Small
	base := env.Path(treeRenameName, w.Name("it"))

	if err := treePut(ctx, c, "rename.seed_put", base+"/a.txt", size, 1); nil != err {
		return err
	}
	if err := treePatch(ctx, c, "rename.file", base+"/a.txt", "", map[string]any{"name": "b.txt"}); nil != err {
		return err
	}
	if err := treePatch(ctx, c, "rename.file_back", base+"/b.txt", "", map[string]any{"name": "a.txt"}); nil != err {
		return err
	}
	if err := treePatch(ctx, c, "rename.move_file", base+"/a.txt", "", map[string]any{"destination": base + "/sub/deep/a.txt", "create_parents": true}); nil != err {
		return err
	}

	if err := treePut(ctx, c, "rename.seed_put", base+"/other.txt", size, 2); nil != err {
		return err
	}
	if err := treePatch(ctx, c, "rename.move_overwrite", base+"/sub/deep/a.txt", "", map[string]any{"destination": base + "/other.txt", "overwrite": true}); nil != err {
		return err
	}
	if err := treePut(ctx, c, "rename.seed_put", base+"/q.txt", size, 3); nil != err {
		return err
	}
	if err := treePatch(ctx, c, "rename.move_overwrite_query", base+"/q.txt", "overwrite=true", map[string]any{"destination": base + "/other.txt"}); nil != err {
		return err
	}

	if err := treePut(ctx, c, "rename.seed_put", base+"/x.txt", size, 4); nil != err {
		return err
	}
	if err := treePut(ctx, c, "rename.seed_put", base+"/y.txt", size, 5); nil != err {
		return err
	}
	if err := treePatch(ctx, c, "rename.conflict", base+"/y.txt", "", map[string]any{"destination": base + "/x.txt"}, http.StatusConflict); nil != err {
		return err
	}

	files := min(5, treeCount(env))
	for i := 0; i < files; i++ {
		if err := treePut(ctx, c, "rename.dir_seed_put", fmt.Sprintf("%s/dir/f-%d", base, i), size, i); nil != err {
			return err
		}
	}
	if err := treePatch(ctx, c, "rename.dir", base+"/dir", "", map[string]any{"name": "dir2"}); nil != err {
		return err
	}
	if err := treePatch(ctx, c, "rename.move_dir", base+"/dir2", "", map[string]any{"destination": base + "/moved/dir3", "create_parents": true}); nil != err {
		return err
	}
	if err := treePatch(ctx, c, "rename.move_dir_back", base+"/moved/dir3", "", map[string]any{"destination": base + "/dir2"}); nil != err {
		return err
	}
	return treeDeleteTree(ctx, c, "rename.cleanup", base)
}

func treeDeleteStep(ctx context.Context, env *Env, w *Worker) error {
	c := w.Client
	base := env.Path(treeDeleteName, w.Name("t"))
	dirs := []string{base, base + "/a", base + "/a/b", base + "/c"}
	for _, dir := range dirs {
		if err := treeMkdir(ctx, c, "delete.mkdir", dir); nil != err {
			return err
		}
	}
	per := min(3, treeCount(env))
	for _, dir := range dirs {
		for i := 0; i < per; i++ {
			if err := treePut(ctx, c, "delete.seed_put", fmt.Sprintf("%s/f-%d", dir, i), env.Sizes.Small, i); nil != err {
				return err
			}
		}
	}
	if _, err := c.Do(ctx, Request{Op: "delete.dry_run", Method: http.MethodDelete, Path: treeFile(base) + "?dry_run=true&recursive=true", Expect: []int{http.StatusOK}}); nil != err {
		return err
	}
	if _, err := c.Do(ctx, Request{Op: "delete.nonempty_refused", Method: http.MethodDelete, Path: treeFile(base + "/a"), Expect: []int{http.StatusConflict}}); nil != err {
		return err
	}
	if _, err := c.Do(ctx, Request{Op: "delete.file", Method: http.MethodDelete, Path: treeFile(base + "/c/f-0"), Expect: []int{http.StatusOK, http.StatusNoContent}}); nil != err {
		return err
	}
	if _, err := c.Do(ctx, Request{Op: "delete.recursive", Method: http.MethodDelete, Path: treeFile(base) + "?recursive=true", Expect: []int{http.StatusOK}}); nil != err {
		return err
	}
	return nil
}

func treeBatchStep(ctx context.Context, env *Env, w *Worker) error {
	if 0 == w.Iter%2 {
		return treeBatchSync(ctx, env, w)
	}
	return treeBatchJob(ctx, env, w)
}

func treeBatchSync(ctx context.Context, env *Env, w *Worker) error {
	c := w.Client
	base := env.Path(treeBatchName, w.Name("s"))
	for i := 1; 6 >= i; i++ {
		if err := treePut(ctx, c, "batch.seed_put", fmt.Sprintf("%s/s%d", base, i), env.Sizes.Small, i); nil != err {
			return err
		}
	}
	if err := treePut(ctx, c, "batch.seed_put", base+"/sd/inner", env.Sizes.Small, 7); nil != err {
		return err
	}
	const trashIndex = 3
	ops := []map[string]any{
		{"op": "mode", "path": base + "/s1", "mode": "0640"},
		{"op": "rename", "path": base + "/s2", "name": "s2r"},
		{"op": "move", "path": base + "/s3", "destination": base + "/m/s3", "create_parents": true},
		{"op": "trash", "path": base + "/s4"},
		{"op": "delete", "path": base + "/s5"},
		{"op": "delete", "path": base + "/sd", "recursive": true},
		{"op": "move", "path": base + "/s6", "destination": base + "/m/s3", "overwrite": true},
	}
	var out treeBatchResponse
	if err := c.JSON(ctx, Request{
		Op:     "batch.sync",
		Method: http.MethodPost,
		Path:   "/v1/batch",
		JSON:   map[string]any{"mode": "sync", "stop_on_error": false, "ops": ops},
		Expect: []int{http.StatusOK},
	}, &out); nil != err {
		return err
	}
	if len(ops) != len(out.Results) {
		return fmt.Errorf("batch returned %d results for %d ops", len(out.Results), len(ops))
	}
	for _, result := range out.Results {
		if trashIndex == result.Index || "ok" == result.Status {
			continue
		}
		return fmt.Errorf("batch op %d on %s failed: %s %s", result.Index, result.Path, result.Code, result.Message)
	}
	return treeDeleteTree(ctx, c, "batch.cleanup", base)
}

func treeBatchJob(ctx context.Context, env *Env, w *Worker) error {
	c := w.Client
	base := env.Path(treeBatchName, w.Name("j"))
	files := max(2, min(25, treeCount(env)))
	for i := 0; i < files; i++ {
		if err := treePut(ctx, c, "batch.seed_put", fmt.Sprintf("%s/j%d", base, i), env.Sizes.Small, i); nil != err {
			return err
		}
	}
	ops := make([]map[string]any, 0, 3*files+1)
	for i := 0; i < files; i++ {
		ops = append(ops, map[string]any{"op": "mode", "path": fmt.Sprintf("%s/j%d", base, i), "mode": "0640"})
	}
	for i := 0; i < files; i++ {
		ops = append(ops, map[string]any{"op": "rename", "path": fmt.Sprintf("%s/j%d", base, i), "name": fmt.Sprintf("r%d", i)})
	}
	for i := 0; i < files; i++ {
		ops = append(ops, map[string]any{"op": "delete", "path": fmt.Sprintf("%s/r%d", base, i)})
	}
	ops = append(ops, map[string]any{"op": "delete", "path": base, "recursive": true})

	var submit treeJobSubmit
	if err := c.JSON(ctx, Request{
		Op:     "batch.job_submit",
		Method: http.MethodPost,
		Path:   "/v1/batch",
		JSON:   map[string]any{"mode": "job", "stop_on_error": false, "ops": ops},
		Expect: []int{http.StatusAccepted},
	}, &submit); nil != err {
		return err
	}
	if "" == submit.JobID {
		return fmt.Errorf("batch job submit returned no job id")
	}

	deadline := time.Now().Add(treeJobDeadline)
	for time.Now().Before(deadline) {
		var job treeJob
		if err := c.JSON(ctx, Request{Op: "jobs.get", Method: http.MethodGet, Path: "/v1/jobs/" + url.PathEscape(submit.JobID)}, &job); nil != err {
			return err
		}
		switch job.State {
		case "succeeded":
			return nil
		case "partial", "failed", "cancelled":
			detail := ""
			if nil != job.Error {
				detail = ": " + *job.Error
			}
			return fmt.Errorf("batch job %s ended %s%s", submit.JobID, job.State, detail)
		}
		if err := treeSleep(ctx, treeJobPollWait); nil != err {
			return err
		}
	}
	return fmt.Errorf("batch job %s did not finish within %s", submit.JobID, treeJobDeadline)
}
