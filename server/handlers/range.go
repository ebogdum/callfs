package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ebogdum/callfs/metadata"
)

var errRangeNotSatisfiable = errors.New("range not satisfiable")

func parseByteRange(header string, size int64) (offset, length int64, ok bool, err error) {
	spec, found := strings.CutPrefix(strings.TrimSpace(header), "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false, nil
	}
	startText, endText, found := strings.Cut(spec, "-")
	if !found {
		return 0, 0, false, nil
	}
	startText = strings.TrimSpace(startText)
	endText = strings.TrimSpace(endText)

	if startText == "" {
		suffix, parseErr := strconv.ParseInt(endText, 10, 64)
		if parseErr != nil || suffix < 0 {
			return 0, 0, false, nil
		}
		if suffix == 0 || size == 0 {
			return 0, 0, false, errRangeNotSatisfiable
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, suffix, true, nil
	}

	start, parseErr := strconv.ParseInt(startText, 10, 64)
	if parseErr != nil || start < 0 {
		return 0, 0, false, nil
	}
	if start >= size {
		return 0, 0, false, errRangeNotSatisfiable
	}

	if endText == "" {
		return start, size - start, true, nil
	}

	end, parseErr := strconv.ParseInt(endText, 10, 64)
	if parseErr != nil || end < start {
		return 0, 0, false, nil
	}
	if end >= size {
		end = size - 1
	}
	return start, end - start + 1, true, nil
}

func etagMatches(headerValue, etag string) bool {
	for _, candidate := range strings.Split(headerValue, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		if strings.TrimPrefix(candidate, "W/") == quotedETag(etag) {
			return true
		}
	}
	return false
}

func quotedETag(etag string) string {
	return "\"" + etag + "\""
}

func contentRangeHeader(offset, n, size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", offset, offset+n-1, size)
}

func handleValidators(w http.ResponseWriter, r *http.Request, md *metadata.Metadata) bool {
	w.Header().Set("Accept-Ranges", "bytes")
	if md.ETag == "" {
		return false
	}
	w.Header().Set("ETag", quotedETag(md.ETag))
	if etagMatches(r.Header.Get("If-None-Match"), md.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

func resolveRange(w http.ResponseWriter, r *http.Request, md *metadata.Metadata) (offset, length int64, partial, ok bool) {
	header := r.Header.Get("Range")
	if header == "" {
		return 0, -1, false, true
	}
	if ifRange := r.Header.Get("If-Range"); ifRange != "" && (md.ETag == "" || ifRange != quotedETag(md.ETag)) {
		return 0, -1, false, true
	}
	offset, length, partial, err := parseByteRange(header, md.Size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", md.Size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return 0, 0, false, false
	}
	if !partial {
		return 0, -1, false, true
	}
	return offset, length, true, true
}
