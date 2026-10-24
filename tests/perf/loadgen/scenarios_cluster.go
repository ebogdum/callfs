package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	clusterGroup         = "cluster"
	clusterLargeSeeds    = 2
	clusterPageSize      = 50
	clusterJobPollEvery  = 25 * time.Millisecond
	clusterJobPollRounds = 800
	clusterRangeCap      = int64(1 << 20)
	clusterErasureHeader = "X-CallFS-Erasure"
)

type clusterErasureList struct {
	Items []struct {
		Path string `json:"path"`
	} `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type clusterErasureDetail struct {
	Shards []struct {
		Index int `json:"index"`
	} `json:"shards"`
}

type clusterJobView struct {
	State string `json:"state"`
}

type clusterJobAccepted struct {
	JobID string `json:"job_id"`
}

func init() {
	register(Scenario{
		Name:     "erasure-write-read",
		Group:    clusterGroup,
		Needs:    func(f Features) bool { return f.Erasure },
		Setup:    erasureWriteReadSetup,
		Step:     erasureWriteReadStep,
		Teardown: func(ctx context.Context, env *Env) error { return clusterTeardown(ctx, env, "erasure-write-read") },
	})
	register(Scenario{
		Name:     "erasure-ops",
		Group:    clusterGroup,
		Needs:    func(f Features) bool { return f.Erasure },
		Setup:    erasureOpsSetup,
		Step:     erasureOpsStep,
		Teardown: func(ctx context.Context, env *Env) error { return clusterTeardown(ctx, env, "erasure-ops") },
	})
	register(Scenario{
		Name:     "replication-writes",
		Group:    clusterGroup,
		Needs:    func(f Features) bool { return f.Replication },
		Setup:    replicationWritesSetup,
		Step:     replicationWritesStep,
		Teardown: func(ctx context.Context, env *Env) error { return clusterTeardown(ctx, env, "replication-writes") },
	})
	register(Scenario{
		Name:     "cluster-cross-instance",
		Group:    clusterGroup,
		Needs:    func(f Features) bool { return f.Cluster },
		Setup:    crossInstanceSetup,
		Step:     crossInstanceStep,
		Teardown: func(ctx context.Context, env *Env) error { return clusterTeardown(ctx, env, "cluster-cross-instance") },
	})
	register(Scenario{
		Name:     "cluster-writes",
		Group:    clusterGroup,
		Needs:    func(f Features) bool { return f.Cluster },
		Setup:    clusterWritesSetup,
		Step:     clusterWritesStep,
		Teardown: func(ctx context.Context, env *Env) error { return clusterTeardown(ctx, env, "cluster-writes") },
	})
}

func clusterFile(p string) string {
	return "/v1/files" + p
}

func clusterMkdir(ctx context.Context, c *Client, op, p string) error {
	_, err := c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodPost,
		Path:   clusterFile(p) + "/",
		Expect: []int{http.StatusOK, http.StatusCreated},
	})
	if nil != err {
		return fmt.Errorf("mkdir %s: %w", p, err)
	}
	return nil
}

func clusterWrite(ctx context.Context, c *Client, op, p string, erasure bool, size int64, seed int) (Response, error) {
	if erasure {
		return c.Do(ctx, Request{
			Op:     op,
			Method: http.MethodPost,
			Path:   clusterFile(p),
			Body:   Payload(size, seed),
			Size:   size,
			Header: map[string]string{clusterErasureHeader: "true"},
			Expect: []int{http.StatusCreated},
		})
	}
	return c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodPut,
		Path:   clusterFile(p),
		Body:   Payload(size, seed),
		Size:   size,
		Expect: []int{http.StatusOK, http.StatusCreated},
	})
}

func clusterRead(ctx context.Context, c *Client, op, p string) error {
	_, err := c.Do(ctx, Request{
		Op:      op,
		Method:  http.MethodGet,
		Path:    clusterFile(p),
		Discard: true,
		Expect:  []int{http.StatusOK},
	})
	return err
}

func clusterRangeRead(ctx context.Context, c *Client, w *Worker, op, p string, size int64) error {
	span := size / 4
	if clusterRangeCap < span {
		span = clusterRangeCap
	}
	if 1 > span {
		span = 1
	}
	room := size - span
	if 0 > room {
		room = 0
	}
	offset := int64(w.Pick(int(room) + 1))
	end := offset + span - 1
	if size-1 < end {
		end = size - 1
	}
	_, err := c.Do(ctx, Request{
		Op:      op,
		Method:  http.MethodGet,
		Path:    clusterFile(p),
		Header:  map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", offset, end)},
		Discard: true,
		Expect:  []int{http.StatusPartialContent},
	})
	return err
}

func clusterDelete(ctx context.Context, c *Client, op, p string) error {
	_, err := c.Do(ctx, Request{
		Op:     op,
		Method: http.MethodDelete,
		Path:   clusterFile(p),
		Expect: []int{http.StatusOK, http.StatusNoContent},
	})
	return err
}

func clusterSeed(ctx context.Context, env *Env, scenario string, erasure bool) error {
	if err := clusterMkdir(ctx, env.Admin, "setup.mkdir", env.Path(scenario, "seed")); nil != err {
		return err
	}
	count := env.Sizes.Files
	if 1 > count {
		count = 1
	}
	paths := make([]string, 0, count)
	for i := 0; i < count; i++ {
		p := env.Path(scenario, "seed", fmt.Sprintf("f%05d", i))
		if _, err := clusterWrite(ctx, env.Admin, "setup.put", p, erasure, env.Sizes.Small, i); nil != err {
			return fmt.Errorf("seed %s: %w", p, err)
		}
		paths = append(paths, p)
	}
	large := make([]string, 0, clusterLargeSeeds)
	if 0 < env.Sizes.Large {
		for i := 0; i < clusterLargeSeeds; i++ {
			p := env.Path(scenario, "seed", fmt.Sprintf("large%d", i))
			if _, err := clusterWrite(ctx, env.Admin, "setup.put_large", p, erasure, env.Sizes.Large, i); nil != err {
				return fmt.Errorf("seed large %s: %w", p, err)
			}
			large = append(large, p)
		}
	}
	env.Put(scenario+".seed", paths)
	env.Put(scenario+".large", large)
	return nil
}

func clusterSeeded(env *Env, scenario, kind string) ([]string, error) {
	value, found := env.Get(scenario + "." + kind)
	if !found {
		return nil, fmt.Errorf("%s %s list is not seeded", scenario, kind)
	}
	paths, ok := value.([]string)
	if !ok {
		return nil, fmt.Errorf("%s %s list has unexpected type", scenario, kind)
	}
	return paths, nil
}

func clusterTeardown(ctx context.Context, env *Env, scenario string) error {
	clients := append([]*Client{env.Admin}, env.Peers...)
	var failures []error
	for _, c := range clients {
		_, err := c.Do(ctx, Request{
			Op:     "teardown.delete_tree",
			Method: http.MethodDelete,
			Path:   clusterFile(env.Path(scenario)) + "/?recursive=true",
			Expect: []int{http.StatusOK, http.StatusNoContent, http.StatusNotFound},
		})
		if nil != err {
			failures = append(failures, fmt.Errorf("teardown via %s: %w", c.Name, err))
		}
	}
	return errors.Join(failures...)
}

func clusterSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func clusterAwaitJob(ctx context.Context, c *Client, op, jobID string) (string, error) {
	for i := 0; i < clusterJobPollRounds; i++ {
		var job clusterJobView
		err := c.JSON(ctx, Request{
			Op:     op,
			Method: http.MethodGet,
			Path:   "/v1/jobs/" + url.PathEscape(jobID),
			Expect: []int{http.StatusOK},
		}, &job)
		if nil != err {
			return "", err
		}
		switch job.State {
		case "succeeded", "partial", "failed", "cancelled":
			return job.State, nil
		}
		if sleepErr := clusterSleep(ctx, clusterJobPollEvery); nil != sleepErr {
			return "", sleepErr
		}
	}
	return "", fmt.Errorf("job %s did not finish", jobID)
}

func erasureWriteReadSetup(ctx context.Context, env *Env) error {
	if err := clusterSeed(ctx, env, "erasure-write-read", true); nil != err {
		return err
	}
	return clusterMkdir(ctx, env.Admin, "setup.mkdir", env.Path("erasure-write-read", "w"))
}

func erasureWriteReadStep(ctx context.Context, env *Env, w *Worker) error {
	const scenario = "erasure-write-read"
	seeded, err := clusterSeeded(env, scenario, "seed")
	if nil != err {
		return err
	}
	large, err := clusterSeeded(env, scenario, "large")
	if nil != err {
		return err
	}
	c := w.Client
	own := env.Path(scenario, "w", w.Name("e"))
	if _, err := clusterWrite(ctx, c, "erasure.put_small", own, true, env.Sizes.Small, w.Iter); nil != err {
		return fmt.Errorf("erasure put: %w", err)
	}
	if err := clusterRead(ctx, c, "erasure.get_full", own); nil != err {
		return fmt.Errorf("erasure get own: %w", err)
	}
	if err := clusterRangeRead(ctx, c, w, "erasure.get_range", own, env.Sizes.Small); nil != err {
		return fmt.Errorf("erasure range own: %w", err)
	}
	if 0 < len(seeded) {
		target := seeded[w.Pick(len(seeded))]
		if err := clusterRead(ctx, c, "erasure.get_seeded", target); nil != err {
			return fmt.Errorf("erasure get seeded: %w", err)
		}
		if err := clusterRangeRead(ctx, c, w, "erasure.get_range_seeded", target, env.Sizes.Small); nil != err {
			return fmt.Errorf("erasure range seeded: %w", err)
		}
	}
	if 0 < len(large) {
		target := large[w.Pick(len(large))]
		if err := clusterRangeRead(ctx, c, w, "erasure.get_range_large", target, env.Sizes.Large); nil != err {
			return fmt.Errorf("erasure range large: %w", err)
		}
		if 0 == w.Iter%10 {
			if err := clusterRead(ctx, c, "erasure.get_full_large", target); nil != err {
				return fmt.Errorf("erasure get large: %w", err)
			}
		}
	}
	if 0 < env.Sizes.Large && 0 == w.Iter%50 {
		bigPath := env.Path(scenario, "w", w.Name("big"))
		if _, err := clusterWrite(ctx, c, "erasure.put_large", bigPath, true, env.Sizes.Large, w.Iter); nil != err {
			return fmt.Errorf("erasure put large: %w", err)
		}
		if err := clusterDelete(ctx, c, "erasure.delete_large", bigPath); nil != err {
			return fmt.Errorf("erasure delete large: %w", err)
		}
	}
	if err := clusterDelete(ctx, c, "erasure.delete", own); nil != err {
		return fmt.Errorf("erasure delete: %w", err)
	}
	return nil
}

func erasureOpsSetup(ctx context.Context, env *Env) error {
	return clusterSeed(ctx, env, "erasure-ops", true)
}

func erasureOpsStep(ctx context.Context, env *Env, w *Worker) error {
	const scenario = "erasure-ops"
	seeded, err := clusterSeeded(env, scenario, "seed")
	if nil != err {
		return err
	}
	if 0 == len(seeded) {
		return errors.New("erasure-ops has no seeded files")
	}
	c := w.Client
	target := seeded[w.Pick(len(seeded))]
	prefix := url.QueryEscape(env.Path(scenario, "seed"))

	var page clusterErasureList
	listPath := fmt.Sprintf("/v1/erasure/files?prefix=%s&limit=%d", prefix, clusterPageSize)
	if err := c.JSON(ctx, Request{Op: "erasure.list", Method: http.MethodGet, Path: listPath, Expect: []int{http.StatusOK}}, &page); nil != err {
		return fmt.Errorf("erasure list: %w", err)
	}
	if nil != page.NextCursor {
		next := listPath + "&cursor=" + url.QueryEscape(*page.NextCursor)
		if err := c.JSON(ctx, Request{Op: "erasure.list_page2", Method: http.MethodGet, Path: next, Expect: []int{http.StatusOK}}, &clusterErasureList{}); nil != err {
			return fmt.Errorf("erasure list page 2: %w", err)
		}
	}
	healthPath := listPath + "&health=ok"
	if err := c.JSON(ctx, Request{Op: "erasure.list_health", Method: http.MethodGet, Path: healthPath, Expect: []int{http.StatusOK}}, &clusterErasureList{}); nil != err {
		return fmt.Errorf("erasure list health: %w", err)
	}

	var detail clusterErasureDetail
	if err := c.JSON(ctx, Request{Op: "erasure.detail", Method: http.MethodGet, Path: "/v1/erasure/files" + target, Expect: []int{http.StatusOK}}, &detail); nil != err {
		return fmt.Errorf("erasure detail: %w", err)
	}
	if _, err := c.Do(ctx, Request{Op: "erasure.manifest", Method: http.MethodGet, Path: "/v1/erasure/manifest" + target, Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("erasure manifest: %w", err)
	}
	if _, err := c.Do(ctx, Request{Op: "erasure.verify", Method: http.MethodPost, Path: "/v1/erasure/verify" + target, Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("erasure verify: %w", err)
	}
	if 0 < len(detail.Shards) {
		index := detail.Shards[w.Pick(len(detail.Shards))].Index
		_, err := c.Do(ctx, Request{
			Op:      "erasure.shard",
			Method:  http.MethodGet,
			Path:    "/v1/shards" + target + "/" + strconv.Itoa(index),
			Discard: true,
			Expect:  []int{http.StatusOK},
		})
		if nil != err {
			return fmt.Errorf("erasure shard: %w", err)
		}
	}
	if err := clusterRead(ctx, c, "erasure.get_full", target); nil != err {
		return fmt.Errorf("erasure get: %w", err)
	}
	if 0 == w.Iter%5 {
		if _, err := env.Admin.Do(ctx, Request{Op: "erasure.admin_summary", Method: http.MethodGet, Path: "/v1/admin/erasure", Expect: []int{http.StatusOK}}); nil != err {
			return fmt.Errorf("erasure admin summary: %w", err)
		}
	}
	if 0 == w.Iter%10 {
		var accepted clusterJobAccepted
		err := env.Admin.JSON(ctx, Request{Op: "erasure.repair", Method: http.MethodPost, Path: "/v1/erasure/repair" + target, Expect: []int{http.StatusAccepted}}, &accepted)
		if nil != err {
			return fmt.Errorf("erasure repair: %w", err)
		}
		state, err := clusterAwaitJob(ctx, env.Admin, "erasure.repair_poll", accepted.JobID)
		if nil != err {
			return fmt.Errorf("erasure repair job: %w", err)
		}
		if "failed" == state || "cancelled" == state {
			return fmt.Errorf("erasure repair job %s ended %s", accepted.JobID, state)
		}
	}
	return nil
}

func replicationWritesSetup(ctx context.Context, env *Env) error {
	if err := clusterSeed(ctx, env, "replication-writes", false); nil != err {
		return err
	}
	return clusterMkdir(ctx, env.Admin, "setup.mkdir", env.Path("replication-writes", "w"))
}

func replicationWritesStep(ctx context.Context, env *Env, w *Worker) error {
	const scenario = "replication-writes"
	c := w.Client
	own := env.Path(scenario, "w", w.Name("r"))
	created, err := c.Do(ctx, Request{
		Op:     "replication.put_new",
		Method: http.MethodPut,
		Path:   clusterFile(own),
		Body:   Payload(env.Sizes.Small, w.Iter),
		Size:   env.Sizes.Small,
		Expect: []int{http.StatusCreated},
	})
	if nil != err {
		return fmt.Errorf("replication put new: %w", err)
	}
	if _, err := clusterWrite(ctx, c, "replication.put_overwrite", own, false, env.Sizes.Small, w.Iter+1); nil != err {
		return fmt.Errorf("replication overwrite: %w", err)
	}
	etag := created.Header.Get("ETag")
	if "" != etag {
		latest, statErr := c.Do(ctx, Request{Op: "replication.head", Method: http.MethodHead, Path: clusterFile(own), Expect: []int{http.StatusOK}})
		if nil != statErr {
			return fmt.Errorf("replication head: %w", statErr)
		}
		if current := latest.Header.Get("ETag"); "" != current {
			_, err := c.Do(ctx, Request{
				Op:     "replication.put_if_match",
				Method: http.MethodPut,
				Path:   clusterFile(own),
				Body:   Payload(env.Sizes.Small, w.Iter+2),
				Size:   env.Sizes.Small,
				Header: map[string]string{"If-Match": current},
				Expect: []int{http.StatusOK},
			})
			if nil != err {
				return fmt.Errorf("replication put if-match: %w", err)
			}
		}
	}
	if err := clusterRead(ctx, c, "replication.get", own); nil != err {
		return fmt.Errorf("replication get: %w", err)
	}
	if 0 < env.Sizes.Large && 0 == w.Iter%40 {
		bigPath := env.Path(scenario, "w", w.Name("big"))
		if _, err := clusterWrite(ctx, c, "replication.put_large", bigPath, false, env.Sizes.Large, w.Iter); nil != err {
			return fmt.Errorf("replication put large: %w", err)
		}
		if err := clusterDelete(ctx, c, "replication.delete_large", bigPath); nil != err {
			return fmt.Errorf("replication delete large: %w", err)
		}
	}
	if 0 == w.Iter%10 {
		if _, err := env.Admin.Do(ctx, Request{Op: "replication.admin_status", Method: http.MethodGet, Path: "/v1/admin/replication", Expect: []int{http.StatusOK}}); nil != err {
			return fmt.Errorf("replication admin status: %w", err)
		}
	}
	if err := clusterDelete(ctx, c, "replication.delete", own); nil != err {
		return fmt.Errorf("replication delete: %w", err)
	}
	return nil
}

func crossInstanceSetup(ctx context.Context, env *Env) error {
	const scenario = "cluster-cross-instance"
	if 0 == len(env.Peers) {
		return errors.New("cluster-cross-instance needs at least one peer (-peers)")
	}
	if err := clusterSeed(ctx, env, scenario, false); nil != err {
		return err
	}
	if err := clusterMkdir(ctx, env.Admin, "setup.mkdir", env.Path(scenario, "a")); nil != err {
		return err
	}
	for i, peer := range env.Peers {
		dir := env.Path(scenario, "b"+strconv.Itoa(i))
		if err := clusterMkdir(ctx, peer, "setup.mkdir", dir); nil != err {
			return err
		}
		for j := 0; j < 8; j++ {
			p := dir + fmt.Sprintf("/seed%d", j)
			if _, err := clusterWrite(ctx, peer, "setup.put", p, false, env.Sizes.Small, j); nil != err {
				return fmt.Errorf("seed on peer %s: %w", peer.Name, err)
			}
		}
	}
	return nil
}

func crossInstanceStep(ctx context.Context, env *Env, w *Worker) error {
	const scenario = "cluster-cross-instance"
	seeded, err := clusterSeeded(env, scenario, "seed")
	if nil != err {
		return err
	}
	index := w.Pick(len(env.Peers))
	peer := env.Peers[index]
	peerDir := env.Path(scenario, "b"+strconv.Itoa(index))

	if 0 < len(seeded) {
		target := seeded[w.Pick(len(seeded))]
		if err := clusterRead(ctx, peer, "cluster.peer_get", target); nil != err {
			return fmt.Errorf("peer get: %w", err)
		}
		if err := clusterRangeRead(ctx, peer, w, "cluster.peer_get_range", target, env.Sizes.Small); nil != err {
			return fmt.Errorf("peer range: %w", err)
		}
		if _, err := peer.Do(ctx, Request{Op: "cluster.peer_head", Method: http.MethodHead, Path: clusterFile(target), Expect: []int{http.StatusOK}}); nil != err {
			return fmt.Errorf("peer head: %w", err)
		}
		if _, err := peer.Do(ctx, Request{Op: "cluster.peer_stat", Method: http.MethodGet, Path: "/v1/stat" + target, Expect: []int{http.StatusOK}}); nil != err {
			return fmt.Errorf("peer stat: %w", err)
		}
	}
	if _, err := peer.Do(ctx, Request{Op: "cluster.peer_list", Method: http.MethodGet, Path: "/v1/directories" + env.Path(scenario, "seed"), Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("peer list: %w", err)
	}
	pageQuery := fmt.Sprintf("/v1/directories%s?limit=%d", env.Path(scenario, "seed"), clusterPageSize)
	if _, err := peer.Do(ctx, Request{Op: "cluster.peer_list_page", Method: http.MethodGet, Path: pageQuery, Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("peer list page: %w", err)
	}

	forward := env.Path(scenario, "a", w.Name("f"))
	if _, err := clusterWrite(ctx, env.Admin, "cluster.admin_put", forward, false, env.Sizes.Small, w.Iter); nil != err {
		return fmt.Errorf("admin put: %w", err)
	}
	if err := clusterRead(ctx, peer, "cluster.peer_get_of_admin", forward); nil != err {
		return fmt.Errorf("peer get of admin file: %w", err)
	}
	if _, err := peer.Do(ctx, Request{Op: "cluster.peer_stat_of_admin", Method: http.MethodGet, Path: "/v1/stat" + forward, Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("peer stat of admin file: %w", err)
	}
	if _, err := clusterWrite(ctx, peer, "cluster.peer_put_overwrite", forward, false, env.Sizes.Small, w.Iter+1); nil != err {
		return fmt.Errorf("peer overwrite of admin file: %w", err)
	}
	if err := clusterRead(ctx, env.Admin, "cluster.admin_get_after_peer_write", forward); nil != err {
		return fmt.Errorf("admin get after peer write: %w", err)
	}
	if err := clusterDelete(ctx, peer, "cluster.peer_delete", forward); nil != err {
		return fmt.Errorf("peer delete of admin file: %w", err)
	}

	reverse := peerDir + "/" + w.Name("r")
	if _, err := clusterWrite(ctx, peer, "cluster.peer_put", reverse, false, env.Sizes.Small, w.Iter); nil != err {
		return fmt.Errorf("peer put: %w", err)
	}
	if err := clusterRead(ctx, env.Admin, "cluster.admin_get_of_peer", reverse); nil != err {
		return fmt.Errorf("admin get of peer file: %w", err)
	}
	if err := clusterRangeRead(ctx, env.Admin, w, "cluster.admin_get_range_of_peer", reverse, env.Sizes.Small); nil != err {
		return fmt.Errorf("admin range of peer file: %w", err)
	}
	if _, err := env.Admin.Do(ctx, Request{Op: "cluster.admin_stat_of_peer", Method: http.MethodGet, Path: "/v1/stat" + reverse, Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("admin stat of peer file: %w", err)
	}
	if _, err := env.Admin.Do(ctx, Request{Op: "cluster.admin_list_of_peer", Method: http.MethodGet, Path: "/v1/directories" + peerDir, Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("admin list of peer dir: %w", err)
	}
	if err := clusterDelete(ctx, env.Admin, "cluster.admin_delete_of_peer", reverse); nil != err {
		return fmt.Errorf("admin delete of peer file: %w", err)
	}
	return nil
}

func clusterTargets(env *Env) []*Client {
	return append([]*Client{env.Admin}, env.Peers...)
}

func clusterWritesSetup(ctx context.Context, env *Env) error {
	return clusterMkdir(ctx, env.Admin, "setup.mkdir", env.Path("cluster-writes", "w"))
}

func clusterWritesStep(ctx context.Context, env *Env, w *Worker) error {
	const scenario = "cluster-writes"
	targets := clusterTargets(env)
	writer := targets[w.Pick(len(targets))]
	reader := targets[w.Pick(len(targets))]

	dir := env.Path(scenario, "w", w.Name("d"))
	if _, err := writer.Do(ctx, Request{Op: "raft.mkdir", Method: http.MethodPost, Path: clusterFile(dir) + "/", Expect: []int{http.StatusCreated}}); nil != err {
		return fmt.Errorf("raft mkdir: %w", err)
	}
	first := dir + "/f1"
	second := dir + "/f2"
	if _, err := writer.Do(ctx, Request{
		Op: "raft.create", Method: http.MethodPut, Path: clusterFile(first),
		Body: Payload(env.Sizes.Small, w.Iter), Size: env.Sizes.Small, Expect: []int{http.StatusCreated},
	}); nil != err {
		return fmt.Errorf("raft create: %w", err)
	}
	if _, err := writer.Do(ctx, Request{
		Op: "raft.create", Method: http.MethodPut, Path: clusterFile(second),
		Body: Payload(env.Sizes.Small, w.Iter+1), Size: env.Sizes.Small, Expect: []int{http.StatusCreated},
	}); nil != err {
		return fmt.Errorf("raft create second: %w", err)
	}
	if _, err := writer.Do(ctx, Request{
		Op: "raft.rename", Method: http.MethodPatch, Path: clusterFile(first),
		JSON: map[string]any{"name": "f1r"}, Expect: []int{http.StatusOK},
	}); nil != err {
		return fmt.Errorf("raft rename: %w", err)
	}
	if _, err := reader.Do(ctx, Request{Op: "raft.stat_other", Method: http.MethodGet, Path: "/v1/stat" + dir + "/f1r", Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("raft stat on other node: %w", err)
	}
	if _, err := reader.Do(ctx, Request{Op: "raft.list_other", Method: http.MethodGet, Path: "/v1/directories" + dir, Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("raft list on other node: %w", err)
	}
	if _, err := writer.Do(ctx, Request{
		Op: "raft.move", Method: http.MethodPatch, Path: clusterFile(second),
		JSON: map[string]any{"destination": dir + "/sub/f2m", "create_parents": true}, Expect: []int{http.StatusOK},
	}); nil != err {
		return fmt.Errorf("raft move: %w", err)
	}
	if _, err := reader.Do(ctx, Request{Op: "raft.stat_moved", Method: http.MethodGet, Path: "/v1/stat" + dir + "/sub/f2m", Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("raft stat moved on other node: %w", err)
	}
	if _, err := writer.Do(ctx, Request{
		Op: "raft.delete_tree", Method: http.MethodDelete, Path: clusterFile(dir) + "/?recursive=true",
		Expect: []int{http.StatusOK, http.StatusNoContent},
	}); nil != err {
		return fmt.Errorf("raft delete tree: %w", err)
	}
	if 0 == w.Iter%5 {
		status := []struct{ op, path string }{
			{"raft.admin_cluster", "/v1/admin/cluster"},
			{"raft.admin_state_stats", "/v1/admin/cluster/state-stats"},
			{"raft.admin_snapshots", "/v1/admin/cluster/snapshots"},
		}
		for _, s := range status {
			if _, err := reader.Do(ctx, Request{Op: s.op, Method: http.MethodGet, Path: s.path, Expect: []int{http.StatusOK}}); nil != err {
				return fmt.Errorf("%s: %w", s.op, err)
			}
		}
		if _, err := reader.Do(ctx, Request{Op: "raft.health", Method: http.MethodGet, Path: "/health", Expect: []int{http.StatusOK}}); nil != err {
			return fmt.Errorf("raft health: %w", err)
		}
	}
	return nil
}
