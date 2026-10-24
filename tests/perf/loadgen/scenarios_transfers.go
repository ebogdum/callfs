package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	uploadsScenario  = "uploads-multipart"
	wsScenario       = "ws-transfer"
	sessionScenario  = "session-browse"
	streamsScenario  = "streams-hold"
	streamsStateKey  = "streams-hold.state"
	wsChunkBytes     = 256 << 10
	wsMaxPayload     = 96 << 20
	wsUploadsPerKey  = 4
	wsUploadsGlobal  = 64
	wsBusyBackoff    = 50 * time.Millisecond
	wsIOWindow       = 120 * time.Second
	uploadPartCap    = 4 << 20
	eventStreamCount = 8
	logStreamCount   = 4
)

var (
	wsGlobalSlots = make(chan struct{}, wsUploadsGlobal)
	errWSBusy     = errors.New("server is at its websocket upload limit")
	wsKeySlots    sync.Map
)

func init() {
	register(Scenario{
		Name:     uploadsScenario,
		Group:    "transfers",
		Setup:    nil,
		Step:     stepUploads,
		Teardown: func(ctx context.Context, env *Env) error { return deleteTree(ctx, env, uploadsScenario) },
	})
	register(Scenario{
		Name:     wsScenario,
		Group:    "transfers",
		Step:     stepWebSocket,
		Teardown: func(ctx context.Context, env *Env) error { return deleteTree(ctx, env, wsScenario) },
	})
	register(Scenario{
		Name:     sessionScenario,
		Group:    "transfers",
		Step:     stepSessionBrowse,
		Teardown: func(ctx context.Context, env *Env) error { return deleteTree(ctx, env, sessionScenario) },
	})
	register(Scenario{
		Name:     streamsScenario,
		Group:    "transfers",
		Setup:    setupStreams,
		Step:     stepStreams,
		Teardown: teardownStreams,
	})
}

func transferSize(env *Env, w *Worker) int64 {
	size := env.Sizes.Small
	if 0 < env.Sizes.Large && 0 == w.Pick(4) {
		size = env.Sizes.Large
	}
	if 1 > size {
		return 1
	}
	return size
}

func uploadPartSize(size int64) int64 {
	part := (size + 2) / 3
	if uploadPartCap < part {
		part = uploadPartCap
	}
	if 1 > part {
		return 1
	}
	return part
}

func stepUploads(ctx context.Context, env *Env, w *Worker) error {
	c := w.Client
	path := env.Path(uploadsScenario, fmt.Sprintf("w%d", w.ID), w.Name("f"))
	size := transferSize(env, w)
	partSize := uploadPartSize(size)
	abort := 0 == w.Pick(5)

	var created struct {
		UploadID string `json:"upload_id"`
	}
	err := c.JSON(ctx, Request{
		Op:     "uploads.create",
		Method: http.MethodPost,
		Path:   "/v1/uploads",
		JSON:   map[string]any{"path": path, "size": size, "overwrite": true},
		Expect: []int{http.StatusCreated},
	}, &created)
	if nil != err {
		return fmt.Errorf("create upload: %w", err)
	}
	if "" == created.UploadID {
		return errors.New("create upload: empty upload id")
	}
	uploadPath := "/v1/uploads/" + created.UploadID

	var offset int64
	partIndex := 0
	for offset < size {
		length := partSize
		if length > size-offset {
			length = size - offset
		}
		_, err := c.Do(ctx, Request{
			Op:     "uploads.put_part",
			Method: http.MethodPut,
			Path:   uploadPath,
			Body:   Payload(length, w.ID+partIndex),
			Size:   length,
			Header: map[string]string{
				"Content-Range": fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, size),
				"Content-Type":  "application/octet-stream",
			},
			Expect: []int{http.StatusOK},
		})
		if nil != err {
			return errors.Join(fmt.Errorf("put part %d: %w", partIndex, err), uploadsAbort(ctx, c, uploadPath))
		}
		offset += length
		if 0 == partIndex {
			var status struct {
				Offset int64  `json:"offset"`
				State  string `json:"state"`
			}
			if err := c.JSON(ctx, Request{Op: "uploads.status", Method: http.MethodGet, Path: uploadPath, Expect: []int{http.StatusOK}}, &status); nil != err {
				return errors.Join(fmt.Errorf("upload status: %w", err), uploadsAbort(ctx, c, uploadPath))
			}
			if status.Offset != offset {
				return errors.Join(fmt.Errorf("upload status offset %d, want %d", status.Offset, offset), uploadsAbort(ctx, c, uploadPath))
			}
		}
		partIndex++
		if abort {
			return uploadsAbort(ctx, c, uploadPath)
		}
	}

	if _, err := c.Do(ctx, Request{
		Op:     "uploads.complete",
		Method: http.MethodPost,
		Path:   uploadPath + "/complete",
		Expect: []int{http.StatusOK, http.StatusCreated},
	}); nil != err {
		return errors.Join(fmt.Errorf("complete upload: %w", err), uploadsAbort(ctx, c, uploadPath))
	}
	if _, err := c.Do(ctx, Request{
		Op:     "uploads.cleanup",
		Method: http.MethodDelete,
		Path:   "/v1/files" + path,
		Expect: []int{http.StatusNoContent, http.StatusOK},
	}); nil != err {
		return fmt.Errorf("delete uploaded file: %w", err)
	}
	return nil
}

func uploadsAbort(ctx context.Context, c *Client, uploadPath string) error {
	_, err := c.Do(ctx, Request{
		Op:     "uploads.abort",
		Method: http.MethodDelete,
		Path:   uploadPath,
		Expect: []int{http.StatusNoContent, http.StatusNotFound, http.StatusConflict},
	})
	if nil != err {
		return fmt.Errorf("abort upload: %w", err)
	}
	return nil
}

func wsBaseURL(base string) string {
	return strings.Replace(base, "http", "ws", 1)
}

func wsAcquire(ctx context.Context, key string) (func(), error) {
	value, _ := wsKeySlots.LoadOrStore(key, make(chan struct{}, wsUploadsPerKey))
	perKey := value.(chan struct{})
	select {
	case perKey <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case wsGlobalSlots <- struct{}{}:
	case <-ctx.Done():
		<-perKey
		return nil, ctx.Err()
	}
	return func() {
		<-wsGlobalSlots
		<-perKey
	}, nil
}

func wsDial(ctx context.Context, c *Client, path, query string) (*websocket.Conn, func(), error) {
	dialer := websocket.Dialer{HandshakeTimeout: 30 * time.Second, ReadBufferSize: wsChunkBytes, WriteBufferSize: wsChunkBytes}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.Key())
	conn, resp, err := dialer.DialContext(ctx, wsBaseURL(c.Base)+"/v1/files/ws"+path+"?"+query, header)
	if nil != err {
		if nil != resp && http.StatusServiceUnavailable == resp.StatusCode {
			return nil, nil, errWSBusy
		}
		if nil != resp {
			return nil, nil, fmt.Errorf("websocket dial status %d: %w", resp.StatusCode, err)
		}
		return nil, nil, fmt.Errorf("websocket dial: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	closeAll := func() {
		stop()
		_ = conn.Close()
	}
	if err := conn.SetReadDeadline(time.Now().Add(wsIOWindow)); nil != err {
		closeAll()
		return nil, nil, err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(wsIOWindow)); nil != err {
		closeAll()
		return nil, nil, err
	}
	return conn, closeAll, nil
}

func wsUploadRun(ctx context.Context, c *Client, path string, size int64, streamed bool, seed int) error {
	query := "mode=upload"
	if streamed {
		query += "&size=" + strconv.FormatInt(size, 10)
	}
	conn, closeAll, err := wsDial(ctx, c, path, query)
	if nil != err {
		return err
	}
	defer closeAll()

	buf := make([]byte, wsChunkBytes)
	source := Payload(size, seed)
	remaining := size
	for 0 < remaining {
		chunk := int64(len(buf))
		if chunk > remaining {
			chunk = remaining
		}
		if _, err := io.ReadFull(source, buf[:chunk]); nil != err {
			return fmt.Errorf("generate payload: %w", err)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, buf[:chunk]); nil != err {
			return fmt.Errorf("write frame: %w", err)
		}
		remaining -= chunk
	}
	closeFrame := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	if err := conn.WriteControl(websocket.CloseMessage, closeFrame, time.Now().Add(10*time.Second)); nil != err {
		return fmt.Errorf("write close: %w", err)
	}
	acked := false
	for {
		kind, data, err := conn.ReadMessage()
		if nil != err {
			if acked && websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return nil
			}
			return fmt.Errorf("read upload result (acked=%t): %w", acked, err)
		}
		if websocket.TextMessage == kind && strings.HasPrefix(string(data), "ok:") {
			acked = true
		}
	}
}

func wsDownloadRun(ctx context.Context, c *Client, path string) (int64, error) {
	conn, closeAll, err := wsDial(ctx, c, path, "mode=download")
	if nil != err {
		return 0, err
	}
	defer closeAll()

	var received int64
	declared := int64(-1)
	for {
		kind, data, err := conn.ReadMessage()
		if nil != err {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				if 0 <= declared && declared != received {
					return received, fmt.Errorf("download size %d, declared %d", received, declared)
				}
				return received, nil
			}
			return received, fmt.Errorf("read download: %w", err)
		}
		switch kind {
		case websocket.BinaryMessage:
			received += int64(len(data))
		case websocket.TextMessage:
			text := string(data)
			if strings.HasPrefix(text, "size:") {
				parsed, parseErr := strconv.ParseInt(strings.TrimPrefix(text, "size:"), 10, 64)
				if nil != parseErr {
					return received, fmt.Errorf("parse size frame %q: %w", text, parseErr)
				}
				declared = parsed
			}
		}
	}
}

func stepWebSocket(ctx context.Context, env *Env, w *Worker) error {
	c := w.Client
	size := transferSize(env, w)
	if wsMaxPayload < size {
		size = wsMaxPayload
	}
	path := env.Path(wsScenario, fmt.Sprintf("w%d", w.ID), w.Name("f"))
	streamed := 0 == w.Iter%2
	op := "ws.upload_spool"
	if streamed {
		op = "ws.upload_stream"
	}

	release, err := wsAcquire(ctx, c.Key())
	if nil != err {
		return err
	}
	started := time.Now()
	uploadErr := wsUploadRun(ctx, c, path, size, streamed, w.ID+w.Iter)
	for errors.Is(uploadErr, errWSBusy) && nil == ctx.Err() {
		c.recorder.Record("ws.busy_retry", time.Since(started), 0, nil)
		select {
		case <-ctx.Done():
		case <-time.After(wsBusyBackoff):
		}
		started = time.Now()
		uploadErr = wsUploadRun(ctx, c, path, size, streamed, w.ID+w.Iter)
	}
	if nil == uploadErr || nil == ctx.Err() {
		c.recorder.Record(op, time.Since(started), size, uploadErr)
	}
	release()
	if nil != uploadErr {
		return fmt.Errorf("%s: %w", op, uploadErr)
	}

	started = time.Now()
	received, downloadErr := wsDownloadRun(ctx, c, path)
	if nil == downloadErr || nil == ctx.Err() {
		c.recorder.Record("ws.download", time.Since(started), received, downloadErr)
	}
	if nil != downloadErr {
		return fmt.Errorf("ws.download: %w", downloadErr)
	}
	if received != size {
		return fmt.Errorf("ws.download returned %d bytes, want %d", received, size)
	}

	if _, err := c.Do(ctx, Request{
		Op:     "ws.cleanup",
		Method: http.MethodDelete,
		Path:   "/v1/files" + path,
		Expect: []int{http.StatusNoContent, http.StatusOK},
	}); nil != err {
		return fmt.Errorf("delete ws file: %w", err)
	}
	return nil
}

func stepSessionBrowse(ctx context.Context, env *Env, w *Worker) error {
	session, err := w.Client.Clone(w.Name("sess"))
	if nil != err {
		return fmt.Errorf("clone client: %w", err)
	}
	defer session.http.CloseIdleConnections()
	if err := session.Login(ctx); nil != err {
		return fmt.Errorf("login: %w", err)
	}
	workErr := sessionWork(ctx, env, w, session)
	_, logoutErr := session.Do(ctx, Request{
		Op:     "session.logout",
		Method: http.MethodDelete,
		Path:   "/v1/session",
		Expect: []int{http.StatusNoContent, http.StatusOK},
	})
	if nil != logoutErr {
		logoutErr = fmt.Errorf("logout: %w", logoutErr)
	}
	return errors.Join(workErr, logoutErr)
}

func sessionWork(ctx context.Context, env *Env, w *Worker, session *Client) error {
	dir := env.Path(sessionScenario, fmt.Sprintf("w%d", w.ID))
	file := dir + "/" + w.Name("f")
	size := env.Sizes.Small
	if 1 > size {
		size = 1
	}

	var me struct {
		UserID string `json:"user_id"`
	}
	if err := session.JSON(ctx, Request{Op: "session.me", Method: http.MethodGet, Path: "/v1/me", Expect: []int{http.StatusOK}}, &me); nil != err {
		return fmt.Errorf("me: %w", err)
	}
	if _, err := session.Do(ctx, Request{Op: "session.capabilities", Method: http.MethodGet, Path: "/v1/capabilities", Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("capabilities: %w", err)
	}
	if _, err := session.Do(ctx, Request{
		Op:     "session.write",
		Method: http.MethodPut,
		Path:   "/v1/files" + file,
		Body:   Payload(size, w.ID),
		Size:   size,
		Header: map[string]string{"Content-Type": "application/octet-stream"},
		Expect: []int{http.StatusOK, http.StatusCreated},
	}); nil != err {
		return fmt.Errorf("csrf write: %w", err)
	}
	if _, err := session.Do(ctx, Request{Op: "session.list", Method: http.MethodGet, Path: "/v1/directories" + dir + "/", Expect: []int{http.StatusOK}}); nil != err {
		return fmt.Errorf("list directory: %w", err)
	}
	if 0 == w.Pick(4) {
		if _, err := session.Do(ctx, Request{
			Op:     "session.write_bad_csrf",
			Method: http.MethodPut,
			Path:   "/v1/files" + file,
			Body:   Payload(size, w.ID),
			Size:   size,
			Header: map[string]string{csrfHeader: "invalid-token", "Content-Type": "application/octet-stream"},
			Expect: []int{http.StatusForbidden},
		}); nil != err {
			return fmt.Errorf("bad csrf write: %w", err)
		}
	}
	if _, err := session.Do(ctx, Request{
		Op:     "session.delete",
		Method: http.MethodDelete,
		Path:   "/v1/files" + file,
		Expect: []int{http.StatusNoContent, http.StatusOK},
	}); nil != err {
		return fmt.Errorf("csrf delete: %w", err)
	}
	return nil
}

type streamHub struct {
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	recorder atomic.Pointer[Recorder]
}

type stampedEvent struct {
	At   string `json:"at"`
	Time string `json:"time"`
}

func setupStreams(ctx context.Context, env *Env) error {
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	hub := &streamHub{cancel: cancel}
	hub.recorder.Store(env.Admin.recorder)
	targets := make([]string, 0, eventStreamCount+logStreamCount)
	for i := 0; i < eventStreamCount; i++ {
		targets = append(targets, "/v1/admin/events")
	}
	for i := 0; i < logStreamCount; i++ {
		targets = append(targets, "/v1/admin/logs/stream")
	}
	for _, target := range targets {
		body, err := openStream(streamCtx, env.Admin, target)
		if nil != err {
			cancel()
			hub.wg.Wait()
			return fmt.Errorf("open %s: %w", target, err)
		}
		hub.wg.Add(1)
		go func(target string, body io.ReadCloser) {
			defer hub.wg.Done()
			defer body.Close()
			drainStream(streamCtx, hub, target, body)
		}(target, body)
	}
	env.Put(streamsStateKey, hub)
	return nil
}

func openStream(ctx context.Context, c *Client, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if nil != err {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key())
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	resp, err := client.Do(req)
	if nil != err {
		return nil, err
	}
	if http.StatusOK != resp.StatusCode {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w %d: %s", errUnexpectedStatus, resp.StatusCode, snippet(data))
	}
	return resp.Body, nil
}

func drainStream(ctx context.Context, hub *streamHub, target string, body io.Reader) {
	reader := bufio.NewReaderSize(body, 64<<10)
	var data strings.Builder
	name := ""
	for {
		line, err := reader.ReadString('\n')
		if nil != err {
			if nil == ctx.Err() {
				hub.recorder.Load().Record("streams.event", 0, 0, fmt.Errorf("stream %s ended: %w", target, err))
			}
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case "" == line:
			if 0 < data.Len() || "" != name {
				recordStreamEvent(hub, name, data.String())
			}
			data.Reset()
			name = ""
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
}

func recordStreamEvent(hub *streamHub, name, payload string) {
	var latency time.Duration
	var stamped stampedEvent
	if nil == json.Unmarshal([]byte(payload), &stamped) {
		raw := stamped.At
		if "" == raw {
			raw = stamped.Time
		}
		if at, err := time.Parse(time.RFC3339Nano, raw); nil == err {
			latency = time.Since(at)
			if 0 > latency {
				latency = 0
			}
		}
	}
	hub.recorder.Load().Record("streams.event", latency, int64(len(name)+len(payload)), nil)
}

func stepStreams(ctx context.Context, env *Env, w *Worker) error {
	value, ok := env.Get(streamsStateKey)
	if !ok {
		return errors.New("stream state missing")
	}
	hub, ok := value.(*streamHub)
	if !ok {
		return errors.New("stream state has wrong type")
	}
	hub.recorder.Store(env.Admin.recorder)

	c := w.Client
	file := env.Path(streamsScenario, fmt.Sprintf("w%d", w.ID), w.Name("f"))
	size := env.Sizes.Small
	if 1 > size {
		size = 1
	}
	if _, err := c.Do(ctx, Request{
		Op:     "streams.write",
		Method: http.MethodPut,
		Path:   "/v1/files" + file,
		Body:   Payload(size, w.ID),
		Size:   size,
		Header: map[string]string{"Content-Type": "application/octet-stream"},
		Expect: []int{http.StatusOK, http.StatusCreated},
	}); nil != err {
		return fmt.Errorf("write: %w", err)
	}
	if 0 == w.Iter%4 {
		if _, err := c.Do(ctx, Request{Op: "streams.me_events", Method: http.MethodGet, Path: "/v1/me/events?limit=50", Expect: []int{http.StatusOK}}); nil != err {
			return fmt.Errorf("me events: %w", err)
		}
	}
	if _, err := c.Do(ctx, Request{
		Op:     "streams.delete",
		Method: http.MethodDelete,
		Path:   "/v1/files" + file,
		Expect: []int{http.StatusNoContent, http.StatusOK},
	}); nil != err {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

func teardownStreams(ctx context.Context, env *Env) error {
	value, ok := env.Get(streamsStateKey)
	if ok {
		if hub, typed := value.(*streamHub); typed {
			hub.cancel()
			hub.wg.Wait()
		}
	}
	return deleteTree(ctx, env, streamsScenario)
}
