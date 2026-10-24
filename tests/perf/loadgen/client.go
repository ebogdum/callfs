package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"slices"
	"strings"
	"time"
)

const csrfHeader = "X-CallFS-CSRF"

type Client struct {
	Name     string
	Base     string
	key      string
	csrf     string
	session  bool
	http     *http.Client
	recorder *Recorder
}

type Request struct {
	Op      string
	Method  string
	Path    string
	Body    io.Reader
	Size    int64
	JSON    any
	Header  map[string]string
	Expect  []int
	Discard bool
}

type Response struct {
	Status int
	Header http.Header
	Body   []byte
	Bytes  int64
}

var errUnexpectedStatus = errors.New("unexpected status")

func NewClient(name, base, key string, recorder *Recorder, timeout time.Duration) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if nil != err {
		return nil, err
	}
	transport := &http.Transport{
		MaxIdleConns:        512,
		MaxIdleConnsPerHost: 512,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	return &Client{
		Name:     name,
		Base:     strings.TrimRight(base, "/"),
		key:      key,
		http:     &http.Client{Transport: transport, Jar: jar, Timeout: timeout},
		recorder: recorder,
	}, nil
}

func (c *Client) Key() string {
	return c.key
}

func (c *Client) Clone(name string) (*Client, error) {
	clone, err := NewClient(name, c.Base, c.key, c.recorder, c.http.Timeout)
	if nil != err {
		return nil, err
	}
	return clone, nil
}

func (c *Client) Login(ctx context.Context) error {
	resp, err := c.Do(ctx, Request{
		Op:     "session.login",
		Method: http.MethodPost,
		Path:   "/v1/session",
		JSON:   map[string]string{"api_key": c.key},
		Expect: []int{http.StatusOK, http.StatusCreated},
	})
	if nil != err {
		return err
	}
	var body struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(resp.Body, &body); nil != err {
		return fmt.Errorf("decode session: %w", err)
	}
	c.csrf = body.CSRFToken
	c.session = true
	return nil
}

func (c *Client) Do(ctx context.Context, req Request) (Response, error) {
	started := time.Now()
	resp, err := c.exchange(ctx, req)
	if nil != err && nil != ctx.Err() {
		return resp, err
	}
	c.recorder.Record(req.Op, time.Since(started), resp.Bytes, err)
	return resp, err
}

func (c *Client) exchange(ctx context.Context, req Request) (Response, error) {
	body := req.Body
	size := req.Size
	contentType := ""
	if nil != req.JSON {
		encoded, err := json.Marshal(req.JSON)
		if nil != err {
			return Response{}, err
		}
		body = bytes.NewReader(encoded)
		size = int64(len(encoded))
		contentType = "application/json"
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, c.Base+req.Path, body)
	if nil != err {
		return Response{}, err
	}
	if nil != body && 0 < size {
		httpReq.ContentLength = size
	}
	if "" != contentType {
		httpReq.Header.Set("Content-Type", contentType)
	}
	if c.session {
		if "" != c.csrf {
			httpReq.Header.Set(csrfHeader, c.csrf)
		}
	} else if "" != c.key {
		httpReq.Header.Set("Authorization", "Bearer "+c.key)
	}
	for name, value := range req.Header {
		httpReq.Header.Set(name, value)
	}
	httpResp, err := c.http.Do(httpReq)
	if nil != err {
		return Response{}, err
	}
	defer httpResp.Body.Close()
	out := Response{Status: httpResp.StatusCode, Header: httpResp.Header}
	if req.Discard && 300 > httpResp.StatusCode {
		n, copyErr := io.Copy(io.Discard, httpResp.Body)
		out.Bytes = n
		if nil != copyErr {
			return out, copyErr
		}
	} else {
		data, readErr := io.ReadAll(httpResp.Body)
		out.Body = data
		out.Bytes = int64(len(data))
		if nil != readErr {
			return out, readErr
		}
	}
	expect := req.Expect
	if 0 == len(expect) {
		if 400 <= out.Status {
			return out, fmt.Errorf("%w %d on %s %s: %s", errUnexpectedStatus, out.Status, req.Method, req.Path, snippet(out.Body))
		}
		return out, nil
	}
	if !slices.Contains(expect, out.Status) {
		return out, fmt.Errorf("%w %d on %s %s (want %v): %s", errUnexpectedStatus, out.Status, req.Method, req.Path, expect, snippet(out.Body))
	}
	return out, nil
}

func (c *Client) JSON(ctx context.Context, req Request, into any) error {
	resp, err := c.Do(ctx, req)
	if nil != err {
		return err
	}
	if nil == into || 0 == len(resp.Body) {
		return nil
	}
	if err := json.Unmarshal(resp.Body, into); nil != err {
		return fmt.Errorf("decode %s: %w", req.Op, err)
	}
	return nil
}

func snippet(body []byte) string {
	const limit = 200
	text := strings.TrimSpace(string(body))
	if limit < len(text) {
		return text[:limit]
	}
	return text
}

type patternReader struct {
	remaining int64
	seed      byte
}

func Payload(size int64, seed int) io.Reader {
	return &patternReader{remaining: size, seed: byte(seed)}
}

func (p *patternReader) Read(buf []byte) (int, error) {
	if 0 >= p.remaining {
		return 0, io.EOF
	}
	n := int64(len(buf))
	if n > p.remaining {
		n = p.remaining
	}
	for i := int64(0); i < n; i++ {
		buf[i] = p.seed + byte(i*31)
	}
	p.remaining -= n
	return int(n), nil
}
