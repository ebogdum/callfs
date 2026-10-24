package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	shGroup         = "sharing"
	shBatchTrashOps = 5
	shPollAttempts  = 200
	shPollInterval  = 20 * time.Millisecond
	shTrashPageSize = 1000
)

var shExpiries = []int{60, 300, 3600, 86400}

type shLink struct {
	ID    int64  `json:"id"`
	Token string `json:"token"`
}

type shBatchResult struct {
	Results []struct {
		Index   int    `json:"index"`
		Status  string `json:"status"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"results"`
}

type shTrashList struct {
	Items []struct {
		ID           string `json:"id"`
		OriginalPath string `json:"original_path"`
	} `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type shJob struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

func shSlots(env *Env) int {
	if 0 == len(env.Users) {
		return 1
	}
	return len(env.Users)
}

func shSlot(env *Env, w *Worker) int {
	return w.ID % shSlots(env)
}

func shSeedClient(env *Env, slot int) *Client {
	if 0 == len(env.Users) {
		return env.Admin
	}
	return env.Users[slot]
}

func shFilePath(env *Env, scenario string, slot int, name string) string {
	return env.Path(scenario, "u"+strconv.Itoa(slot), name)
}

func shPut(ctx context.Context, c *Client, op, path string, size int64, seed int) error {
	_, err := c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodPut,
		Path:   "/v1/files" + path,
		Body:   Payload(size, seed),
		Size:   size,
		Expect: []int{http.StatusOK, http.StatusCreated},
	})
	if nil != err {
		return fmt.Errorf("put %s: %w", path, err)
	}
	return nil
}

func shSeed(ctx context.Context, env *Env, scenario string, large int) error {
	slots := shSlots(env)
	small := make([][]string, slots)
	bigs := make([]string, slots)
	for slot := 0; slot < slots; slot++ {
		client := shSeedClient(env, slot)
		for i := 0; i < env.Sizes.Files; i++ {
			p := shFilePath(env, scenario, slot, "seed-"+strconv.Itoa(i))
			if err := shPut(ctx, client, "seed.put_small", p, env.Sizes.Small, i); nil != err {
				return err
			}
			small[slot] = append(small[slot], p)
		}
		if 0 < large {
			p := shFilePath(env, scenario, slot, "seed-large")
			if err := shPut(ctx, client, "seed.put_large", p, env.Sizes.Large, slot); nil != err {
				return err
			}
			bigs[slot] = p
		}
	}
	env.Put(scenario+"/small", small)
	env.Put(scenario+"/large", bigs)
	return nil
}

func shPickSmall(env *Env, scenario string, w *Worker) (string, error) {
	value, ok := env.Get(scenario + "/small")
	if !ok {
		return "", fmt.Errorf("%s: seeded files missing", scenario)
	}
	lists, ok := value.([][]string)
	if !ok {
		return "", fmt.Errorf("%s: seeded files have unexpected type", scenario)
	}
	list := lists[shSlot(env, w)]
	if 0 == len(list) {
		return "", fmt.Errorf("%s: no seeded files for worker %d", scenario, w.ID)
	}
	return list[w.Pick(len(list))], nil
}

func shLargePath(env *Env, scenario string, w *Worker) (string, bool) {
	value, ok := env.Get(scenario + "/large")
	if !ok {
		return "", false
	}
	list, ok := value.([]string)
	if !ok {
		return "", false
	}
	p := list[shSlot(env, w)]
	return p, "" != p
}

func shTeardown(ctx context.Context, env *Env, scenario string) error {
	_, err := env.Admin.Do(ctx, Request{
		Op:     "seed.delete_tree",
		Method: http.MethodDelete,
		Path:   "/v1/files" + env.Path(scenario) + "/?recursive=true",
		Expect: []int{http.StatusOK, http.StatusNoContent, http.StatusNotFound},
	})
	if nil != err {
		return fmt.Errorf("delete tree: %w", err)
	}
	return nil
}

func shGenerate(ctx context.Context, c *Client, op, path string, w *Worker) (shLink, error) {
	var link shLink
	err := c.JSON(ctx, Request{
		Op:     op,
		Method: http.MethodPost,
		Path:   "/v1/links/generate",
		JSON:   map[string]any{"path": path, "expiry_seconds": shExpiries[w.Pick(len(shExpiries))]},
		Expect: []int{http.StatusCreated},
	}, &link)
	if nil != err {
		return shLink{}, fmt.Errorf("generate link: %w", err)
	}
	if "" == link.Token {
		return shLink{}, fmt.Errorf("generate link for %s: empty token", path)
	}
	return link, nil
}

func shRevoke(ctx context.Context, c *Client, op string, id int64) error {
	_, err := c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodDelete,
		Path:   "/v1/links/" + strconv.FormatInt(id, 10),
		Expect: []int{http.StatusOK},
	})
	if nil != err {
		return fmt.Errorf("revoke link %d: %w", id, err)
	}
	return nil
}

func shBatchTrash(ctx context.Context, c *Client, op string, paths []string) error {
	ops := make([]map[string]any, 0, len(paths))
	for _, p := range paths {
		ops = append(ops, map[string]any{"op": "trash", "path": p})
	}
	var result shBatchResult
	err := c.JSON(ctx, Request{
		Op:     op,
		Method: http.MethodPost,
		Path:   "/v1/batch",
		JSON:   map[string]any{"ops": ops},
		Expect: []int{http.StatusOK},
	}, &result)
	if nil != err {
		return fmt.Errorf("batch trash: %w", err)
	}
	if len(paths) != len(result.Results) {
		return fmt.Errorf("batch trash: %d results for %d ops", len(result.Results), len(paths))
	}
	for _, r := range result.Results {
		if "ok" != r.Status {
			return fmt.Errorf("batch trash op %d failed: %s %s", r.Index, r.Code, r.Message)
		}
	}
	return nil
}

func shFindTrash(ctx context.Context, c *Client, originalPath string) (string, error) {
	cursor := ""
	for {
		query := url.Values{}
		query.Set("limit", strconv.Itoa(shTrashPageSize))
		if "" != cursor {
			query.Set("cursor", cursor)
		}
		var page shTrashList
		err := c.JSON(ctx, Request{
			Op:     "trash.list",
			Method: http.MethodGet,
			Path:   "/v1/trash?" + query.Encode(),
			Expect: []int{http.StatusOK},
		}, &page)
		if nil != err {
			return "", fmt.Errorf("list trash: %w", err)
		}
		for _, item := range page.Items {
			if originalPath == item.OriginalPath {
				return item.ID, nil
			}
		}
		if nil == page.NextCursor || "" == *page.NextCursor {
			return "", fmt.Errorf("trash item for %s not found", originalPath)
		}
		cursor = *page.NextCursor
	}
}

func shProbe(ctx context.Context, env *Env, path string) error {
	_, err := env.Admin.Do(ctx, Request{
		Op:     "seed.probe",
		Method: http.MethodGet,
		Path:   path,
		Expect: []int{http.StatusOK},
	})
	if nil != err {
		return fmt.Errorf("route %s unavailable (is the feature enabled?): %w", path, err)
	}
	return nil
}

func shWaitJob(ctx context.Context, c *Client, id string) error {
	for attempt := 0; attempt < shPollAttempts; attempt++ {
		var job shJob
		err := c.JSON(ctx, Request{
			Op:     "jobs.status",
			Method: http.MethodGet,
			Path:   "/v1/jobs/" + url.PathEscape(id),
			Expect: []int{http.StatusOK},
		}, &job)
		if nil != err {
			return fmt.Errorf("job status: %w", err)
		}
		switch job.State {
		case "queued", "running", "cancelling":
		default:
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(shPollInterval):
		}
	}
	return fmt.Errorf("job %s did not finish", id)
}

func init() {
	register(Scenario{
		Name:  "links-generate",
		Group: shGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return shSeed(ctx, env, "links-generate", 0)
		},
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			p, err := shPickSmall(env, "links-generate", w)
			if nil != err {
				return err
			}
			link, err := shGenerate(ctx, w.Client, "links.generate", p, w)
			if nil != err {
				return err
			}
			return shRevoke(ctx, w.Client, "links.revoke", link.ID)
		},
		Teardown: func(ctx context.Context, env *Env) error {
			return shTeardown(ctx, env, "links-generate")
		},
	})

	register(Scenario{
		Name:  "links-download",
		Group: shGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return shSeed(ctx, env, "links-download", 1)
		},
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			p, err := shPickSmall(env, "links-download", w)
			if nil != err {
				return err
			}
			if 0 == w.Pick(10) {
				if big, ok := shLargePath(env, "links-download", w); ok {
					p = big
				}
			}
			link, err := shGenerate(ctx, w.Client, "links.generate", p, w)
			if nil != err {
				return err
			}
			downloadPath := "/download/" + url.PathEscape(link.Token)
			if _, err := w.Client.Do(ctx, Request{
				Op:     "links.confirm",
				Method: http.MethodGet,
				Path:   downloadPath,
				Expect: []int{http.StatusOK},
			}); nil != err {
				return fmt.Errorf("confirm download: %w", err)
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:      "links.download",
				Method:  http.MethodPost,
				Path:    downloadPath,
				Expect:  []int{http.StatusOK},
				Discard: true,
			}); nil != err {
				return fmt.Errorf("download: %w", err)
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:     "links.reuse_rejected",
				Method: http.MethodPost,
				Path:   downloadPath,
				Expect: []int{http.StatusGone, http.StatusNotFound},
			}); nil != err {
				return fmt.Errorf("reuse check: %w", err)
			}
			return nil
		},
		Teardown: func(ctx context.Context, env *Env) error {
			return shTeardown(ctx, env, "links-download")
		},
	})

	register(Scenario{
		Name:  "links-manage",
		Group: shGroup,
		Setup: func(ctx context.Context, env *Env) error {
			if err := shProbe(ctx, env, "/v1/links/stats"); nil != err {
				return err
			}
			return shSeed(ctx, env, "links-manage", 0)
		},
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			p, err := shPickSmall(env, "links-manage", w)
			if nil != err {
				return err
			}
			link, err := shGenerate(ctx, w.Client, "links.generate", p, w)
			if nil != err {
				return err
			}
			id := strconv.FormatInt(link.ID, 10)
			if _, err := w.Client.Do(ctx, Request{
				Op:     "links.get",
				Method: http.MethodGet,
				Path:   "/v1/links/" + id,
				Expect: []int{http.StatusOK},
			}); nil != err {
				return fmt.Errorf("get link: %w", err)
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:     "links.list_active",
				Method: http.MethodGet,
				Path:   "/v1/links/?status=active&limit=100",
				Expect: []int{http.StatusOK},
			}); nil != err {
				return fmt.Errorf("list active links: %w", err)
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:     "links.list_by_path",
				Method: http.MethodGet,
				Path:   "/v1/links/?limit=50&path=" + url.QueryEscape(p),
				Expect: []int{http.StatusOK},
			}); nil != err {
				return fmt.Errorf("list links by path: %w", err)
			}
			windows := []string{"1h", "24h", "7d"}
			if _, err := w.Client.Do(ctx, Request{
				Op:     "links.stats",
				Method: http.MethodGet,
				Path:   "/v1/links/stats?window=" + windows[w.Pick(len(windows))],
				Expect: []int{http.StatusOK},
			}); nil != err {
				return fmt.Errorf("link stats: %w", err)
			}
			return shRevoke(ctx, w.Client, "links.revoke", link.ID)
		},
		Teardown: func(ctx context.Context, env *Env) error {
			return shTeardown(ctx, env, "links-manage")
		},
	})

	register(Scenario{
		Name:  "trash-cycle",
		Group: shGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return shProbe(ctx, env, "/v1/trash?limit=1")
		},
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			p := shFilePath(env, "trash-cycle", shSlot(env, w), w.Name("f"))
			if err := shPut(ctx, w.Client, "trash.put", p, env.Sizes.Small, w.Iter); nil != err {
				return err
			}
			if err := shBatchTrash(ctx, w.Client, "trash.move", []string{p}); nil != err {
				return err
			}
			id, err := shFindTrash(ctx, w.Client, p)
			if nil != err {
				return err
			}
			if 0 == w.Iter%2 {
				if _, err := w.Client.Do(ctx, Request{
					Op:     "trash.restore",
					Method: http.MethodPost,
					Path:   "/v1/trash/" + url.PathEscape(id) + "/restore",
					JSON:   map[string]string{"on_conflict": "fail"},
					Expect: []int{http.StatusOK},
				}); nil != err {
					return fmt.Errorf("restore: %w", err)
				}
				if _, err := w.Client.Do(ctx, Request{
					Op:     "trash.cleanup_file",
					Method: http.MethodDelete,
					Path:   "/v1/files" + p,
					Expect: []int{http.StatusNoContent},
				}); nil != err {
					return fmt.Errorf("delete restored file: %w", err)
				}
				return nil
			}
			if _, err := w.Client.Do(ctx, Request{
				Op:     "trash.delete_item",
				Method: http.MethodDelete,
				Path:   "/v1/trash/" + url.PathEscape(id),
				Expect: []int{http.StatusNoContent},
			}); nil != err {
				return fmt.Errorf("delete trash item: %w", err)
			}
			return nil
		},
		Teardown: func(ctx context.Context, env *Env) error {
			return shTeardown(ctx, env, "trash-cycle")
		},
	})

	register(Scenario{
		Name:  "trash-empty",
		Group: shGroup,
		Setup: func(ctx context.Context, env *Env) error {
			return shProbe(ctx, env, "/v1/trash?limit=1")
		},
		Step: func(ctx context.Context, env *Env, w *Worker) error {
			paths := make([]string, 0, shBatchTrashOps)
			for i := 0; i < shBatchTrashOps; i++ {
				p := shFilePath(env, "trash-empty", shSlot(env, w), w.Name("f"+strconv.Itoa(i)))
				if err := shPut(ctx, w.Client, "trash.put", p, env.Sizes.Small, i); nil != err {
					return err
				}
				paths = append(paths, p)
			}
			if err := shBatchTrash(ctx, w.Client, "trash.move_batch", paths); nil != err {
				return err
			}
			var job struct {
				JobID string `json:"job_id"`
			}
			jobID := ""
			err := w.Client.JSON(ctx, Request{
				Op:     "trash.empty",
				Method: http.MethodDelete,
				Path:   "/v1/trash",
				Expect: []int{http.StatusAccepted},
			}, &job)
			if nil != err {
				return fmt.Errorf("empty trash: %w", err)
			}
			jobID = job.JobID
			if "" == jobID {
				return fmt.Errorf("empty trash: no job id")
			}
			return shWaitJob(ctx, w.Client, jobID)
		},
		Teardown: func(ctx context.Context, env *Env) error {
			_, emptyErr := env.Admin.Do(ctx, Request{
				Op:     "seed.empty_trash",
				Method: http.MethodDelete,
				Path:   "/v1/trash",
				Expect: []int{http.StatusAccepted},
			})
			if nil != emptyErr {
				emptyErr = fmt.Errorf("empty trash: %w", emptyErr)
			}
			treeErr := shTeardown(ctx, env, "trash-empty")
			if nil != emptyErr {
				return emptyErr
			}
			return treeErr
		},
	})
}
