package natsbytes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ebogdum/callfs/v2/backends/internalproxy"
	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/metadata"
)

type ClientOptions struct {
	RPCTimeout  time.Duration
	ChunkSize   int
	Window      int
	IdleTimeout time.Duration
}

type Client struct {
	nc      *nats.Conn
	self    string
	timeout time.Duration
	chunk   int
	window  int
	idle    time.Duration
}

var (
	_ internalproxy.Control       = (*Client)(nil)
	_ internalproxy.ByteTransport = (*Client)(nil)
)

func NewClient(nc *nats.Conn, self string, opts ClientOptions) (*Client, error) {
	if nil == nc {
		return nil, errors.New("natsbytes: nats connection is required")
	}
	if 0 >= opts.RPCTimeout {
		opts.RPCTimeout = DefaultRPCTimeout
	}
	if 1 > opts.ChunkSize {
		opts.ChunkSize = DefaultChunkSize
	}
	if 0 >= opts.IdleTimeout {
		opts.IdleTimeout = DefaultIdleTimeout
	}
	return &Client{
		nc:      nc,
		self:    self,
		timeout: opts.RPCTimeout,
		chunk:   chunkLimit(nc, opts.ChunkSize),
		window:  clampWindow(opts.Window, maxWindow),
		idle:    opts.IdleTimeout,
	}, nil
}

func (c *Client) request(ctx context.Context, instanceID, op string, body []byte) (*nats.Msg, error) {
	user := OnBehalfOf(ctx)
	if "" != user && !cluster.ValidOnBehalfOf(user) {
		return nil, cluster.ErrInvalidOnBehalf
	}
	if "" == instanceID {
		return nil, errors.New("natsbytes: instance id is required")
	}
	msg := &nats.Msg{Subject: NodePrefix(instanceID) + op, Data: body, Header: nats.Header{}}
	msg.Header.Set(cluster.HeaderFrom, c.self)
	if "" != user {
		msg.Header.Set(cluster.HeaderOnBehalfOf, user)
	}
	if formatted := cluster.FormatDeadline(cluster.CallDeadline(ctx, c.timeout)); "" != formatted {
		msg.Header.Set(cluster.HeaderDeadline, formatted)
	}
	return msg, nil
}

func (c *Client) call(ctx context.Context, instanceID, op string, req, resp any) error {
	body, err := json.Marshal(req)
	if nil != err {
		return fmt.Errorf("natsbytes: encode %s request: %w", op, err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	msg, err := c.request(ctx, instanceID, op, body)
	if nil != err {
		return err
	}
	reply, err := c.nc.RequestMsgWithContext(ctx, msg)
	if nil != err {
		return transportError(err)
	}
	if err := frameError(reply); nil != err {
		return err
	}
	if nil == resp || 0 == len(reply.Data) {
		return nil
	}
	if err := json.Unmarshal(reply.Data, resp); nil != err {
		return fmt.Errorf("natsbytes: decode %s reply: %w", op, err)
	}
	return nil
}

func transportError(err error) error {
	switch {
	case errors.Is(err, nats.ErrNoResponders):
		return cluster.ErrNoResponders
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, nats.ErrTimeout):
		return cluster.ErrTimeout
	case errors.Is(err, context.Canceled):
		return cluster.ErrCanceled
	case errors.Is(err, nats.ErrConnectionClosed):
		return cluster.ErrClosed
	}
	return cluster.NewError(http.StatusServiceUnavailable, "transport", err.Error())
}

func frameError(m *nats.Msg) error {
	status, err := cluster.ParseStatus(m.Header.Get(cluster.HeaderStatus))
	if nil != err {
		return cluster.NewError(http.StatusBadGateway, "bad_reply", err.Error())
	}
	if 400 > status {
		return nil
	}
	return fromWire(cluster.DecodeError(status, m.Header.Get(cluster.HeaderErrorCode), m.Data))
}

func (c *Client) Stat(ctx context.Context, instanceID, path string) (*metadata.Metadata, error) {
	var info FileInfo
	if err := c.call(ctx, instanceID, OpStat, PathRequest{Path: path}, &info); nil != err {
		return nil, err
	}
	return fromFileInfo(info), nil
}

func (c *Client) List(ctx context.Context, instanceID, path string) ([]*metadata.Metadata, error) {
	children := make([]*metadata.Metadata, 0)
	offset := 0
	for {
		var reply ListReply
		if err := c.call(ctx, instanceID, OpList, ListRequest{Path: path, Offset: offset, Limit: maxListPage}, &reply); nil != err {
			return nil, err
		}
		for _, entry := range reply.Entries {
			if maxListingEntries <= len(children) {
				return nil, fmt.Errorf("directory listing response exceeds %d entries", maxListingEntries)
			}
			children = append(children, fromFileInfo(entry))
		}
		if 0 == reply.Next || reply.Next <= offset {
			return children, nil
		}
		offset = reply.Next
	}
}

func (c *Client) Delete(ctx context.Context, instanceID, path string, recursive bool) error {
	op := OpDelete
	if recursive {
		op = OpDeleteTree
	}
	return c.call(ctx, instanceID, op, DeleteRequest{Path: path, Recursive: recursive}, nil)
}

func (c *Client) Mkdir(ctx context.Context, instanceID, path string) error {
	return c.call(ctx, instanceID, OpMkdir, PathRequest{Path: path}, nil)
}

func (c *Client) Move(ctx context.Context, instanceID, srcPath string, body []byte) error {
	var req MoveRequest
	if err := json.Unmarshal(body, &req); nil != err {
		return internalproxy.NewBadRequestError("invalid move request: " + err.Error())
	}
	req.Source = srcPath
	if "" == strings.Trim(req.Destination, "/") {
		return internalproxy.NewBadRequestError("move request needs a destination")
	}
	return c.call(ctx, instanceID, OpMove, req, nil)
}
