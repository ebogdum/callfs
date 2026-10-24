package natsbytes

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ebogdum/callfs/v2/backends/internalproxy"
	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/metadata"
)

const (
	OpStat       = "stat"
	OpList       = "list"
	OpDelete     = "delete"
	OpDeleteTree = "delete_tree"
	OpMkdir      = "mkdir"
	OpMove       = "move"
	OpOpen       = "open"
	OpCreate     = "create"
	OpUpdate     = "update"

	flowToken = "flow"

	HeaderFrame  = "Callfs-Frame"
	HeaderSeq    = "Callfs-Seq"
	HeaderSize   = "Callfs-Size"
	HeaderFlow   = "Callfs-Flow"
	HeaderCredit = "Callfs-Credit"
	HeaderSHA256 = "Callfs-Sha256"
	HeaderBytes  = "Callfs-Bytes"
	HeaderETag   = "Callfs-Etag"

	frameHead    = "h"
	frameData    = "d"
	frameTrailer = "t"
	frameCredit  = "c"
	frameAccept  = "a"
	frameResult  = "r"
	frameCancel  = "x"

	DefaultChunkSize   = 256 << 10
	DefaultWindow      = 8
	DefaultIdleTimeout = 2 * time.Minute
	DefaultRPCTimeout  = 10 * time.Second

	maxWindow         = 64
	maxListPage       = 1000
	maxReplyBytes     = 768 << 10
	maxListingEntries = 100000
	payloadMargin     = 4096
	fallbackPayload   = 1 << 20

	codeNotFound      = "not_found"
	codeAlreadyExists = "already_exists"
	codePrecondition  = "precondition"
	codeForbidden     = "forbidden"
	codeNotEmpty      = "not_empty"
	codeBadRequest    = "bad_request"
	codeBroken        = "stream_broken"
	codeChecksum      = "checksum"
)

var (
	ErrNotEmpty     = errors.New("directory not empty")
	ErrChecksum     = errors.New("transferred content does not match its sha256")
	ErrStreamBroken = errors.New("transfer stream broken")
)

type onBehalfKey struct{}

func WithOnBehalfOf(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, onBehalfKey{}, user)
}

func OnBehalfOf(ctx context.Context) string {
	user, _ := ctx.Value(onBehalfKey{}).(string)
	return user
}

func BadRequest(message string) error {
	return cluster.NewError(http.StatusBadRequest, codeBadRequest, message)
}

func NodePrefix(node string) string {
	return cluster.SubjectRoot + ".node." + cluster.Token(node) + ".fs."
}

func ServeSubject(node string) string {
	return NodePrefix(node) + ">"
}

type PathRequest struct {
	Path string `json:"path"`
}

type DeleteRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

type ListRequest struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

type ListReply struct {
	Entries []FileInfo `json:"entries"`
	Next    int        `json:"next"`
}

type FileInfo struct {
	Name  string    `json:"name"`
	Path  string    `json:"path"`
	Type  string    `json:"type"`
	Size  int64     `json:"size"`
	Mode  string    `json:"mode"`
	Owner string    `json:"owner"`
	MTime time.Time `json:"mtime"`
	ATime time.Time `json:"atime"`
	CTime time.Time `json:"ctime"`
}

type MoveRequest struct {
	Source        string `json:"source"`
	Destination   string `json:"destination"`
	Backend       string `json:"backend,omitempty"`
	Instance      string `json:"instance,omitempty"`
	Overwrite     bool   `json:"overwrite,omitempty"`
	CreateParents bool   `json:"create_parents,omitempty"`
}

type OpenRequest struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	Window int    `json:"window"`
}

type WriteRequest struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	IfMatch     string `json:"if_match,omitempty"`
	IfNoneMatch bool   `json:"if_none_match,omitempty"`
	Window      int    `json:"window"`
}

func toFileInfo(md *metadata.Metadata, path string) FileInfo {
	return FileInfo{
		Name:  md.Name,
		Path:  path,
		Type:  md.Type,
		Size:  md.Size,
		Mode:  md.Mode,
		Owner: md.Owner,
		MTime: md.MTime,
		ATime: md.ATime,
		CTime: md.CTime,
	}
}

func fromFileInfo(info FileInfo) *metadata.Metadata {
	path := info.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	mode := info.Mode
	if "" == mode {
		mode = "0644"
		if "directory" == info.Type {
			mode = "0755"
		}
	}
	typeName := info.Type
	if "" == typeName {
		typeName = "file"
	}
	return &metadata.Metadata{
		Name:        info.Name,
		Path:        path,
		Type:        typeName,
		Size:        info.Size,
		Mode:        mode,
		Owner:       info.Owner,
		MTime:       info.MTime,
		ATime:       info.ATime,
		CTime:       info.CTime,
		BackendType: "localfs",
	}
}

func toWire(err error) *cluster.Error {
	var wire *cluster.Error
	switch {
	case errors.As(err, &wire):
		return wire
	case errors.Is(err, metadata.ErrNotFound):
		return cluster.NewError(http.StatusNotFound, codeNotFound, err.Error())
	case errors.Is(err, metadata.ErrAlreadyExists):
		return cluster.NewError(http.StatusConflict, codeAlreadyExists, err.Error())
	case errors.Is(err, metadata.ErrPrecondition):
		return cluster.NewError(http.StatusPreconditionFailed, codePrecondition, err.Error())
	case errors.Is(err, metadata.ErrForbidden):
		return cluster.NewError(http.StatusForbidden, codeForbidden, err.Error())
	case errors.Is(err, ErrNotEmpty), errors.Is(err, syscall.ENOTEMPTY):
		return cluster.NewError(http.StatusConflict, codeNotEmpty, err.Error())
	case errors.Is(err, ErrChecksum):
		return cluster.NewError(http.StatusUnprocessableEntity, codeChecksum, err.Error())
	case errors.Is(err, ErrStreamBroken):
		return cluster.NewError(http.StatusBadGateway, codeBroken, err.Error())
	}
	return cluster.ToError(err)
}

func fromWire(err error) error {
	var wire *cluster.Error
	if !errors.As(err, &wire) {
		return err
	}
	switch wire.Code {
	case codeNotFound:
		return metadata.ErrNotFound
	case codeAlreadyExists:
		return metadata.ErrAlreadyExists
	case codePrecondition:
		return metadata.ErrPrecondition
	case codeForbidden:
		return metadata.ErrForbidden
	case codeNotEmpty:
		return internalproxy.ErrRemoteConflict
	case codeBadRequest:
		return internalproxy.NewBadRequestError(wire.Message)
	case codeChecksum:
		return ErrChecksum
	case codeBroken:
		return ErrStreamBroken
	}
	return wire
}

func atoi(value string) (int, bool) {
	parsed, err := strconv.Atoi(value)
	if nil != err {
		return 0, false
	}
	return parsed, true
}

func atoi64(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if nil != err {
		return 0, false
	}
	return parsed, true
}

func clampWindow(requested, ceiling int) int {
	if 1 > requested {
		requested = DefaultWindow
	}
	if 1 > ceiling {
		ceiling = maxWindow
	}
	return min(requested, ceiling, maxWindow)
}

func relative(path string) string {
	trimmed := strings.TrimLeft(path, "/")
	if "" == trimmed {
		return "/"
	}
	return trimmed
}

func absolute(path string) string {
	return "/" + strings.TrimLeft(path, "/")
}
