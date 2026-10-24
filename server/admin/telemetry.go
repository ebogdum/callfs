package admin

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ebogdum/callfs/server/handlers"
	"github.com/ebogdum/callfs/telemetry"
	"go.uber.org/zap"
)

const (
	telemetryDefaultWindow  = time.Hour
	telemetryDefaultStep    = time.Minute
	telemetryMaxStepSeconds = 2678400
)

var telemetryAuditCSVHeader = []string{
	"at", "request_id", "user_id", "method", "route", "path", "op", "status",
	"bytes_in", "bytes_out", "duration_ms", "instance_id", "client",
}

type TransferSource interface {
	ActiveTransfers() []Transfer
}

type Transfer struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Kind       string    `json:"kind"`
	Path       string    `json:"path"`
	BytesDone  int64     `json:"bytes_done"`
	BytesTotal *int64    `json:"bytes_total"`
	StartedAt  time.Time `json:"started_at"`
	Client     string    `json:"client"`
}

func (a *API) Metrics(w http.ResponseWriter, r *http.Request) {
	if nil == a.Deps.Sampler {
		a.telemetryUnavailable(w, "metrics sampler")
		return
	}
	wanted := telemetrySplitList(r.URL.Query().Get("names"))
	keep := make(map[string]bool, len(wanted))
	for _, name := range wanted {
		keep[name] = true
	}
	families := a.Deps.Sampler.Families()
	selected := make([]telemetry.FamilySnapshot, 0, len(families))
	for _, family := range families {
		if 0 < len(keep) && !keep[family.Name] {
			continue
		}
		selected = append(selected, family)
	}
	handlers.SendJSONResponse(w, struct {
		GeneratedAt time.Time                  `json:"generated_at"`
		Families    []telemetry.FamilySnapshot `json:"families"`
	}{GeneratedAt: time.Now().UTC(), Families: selected})
}

func (a *API) MetricSeries(w http.ResponseWriter, r *http.Request) {
	if nil == a.Deps.Sampler {
		a.telemetryUnavailable(w, "metrics sampler")
		return
	}
	query := r.URL.Query()
	name := strings.TrimSpace(query.Get("name"))
	if "" == name {
		a.telemetryError(w, fmt.Errorf("%w: name is required", telemetry.ErrInvalidQuery))
		return
	}
	measure := telemetry.Measure(query.Get("measure"))
	if "" == measure {
		measure = telemetry.MeasureValue
	}
	filters, err := telemetryParseFilters(query["filter"])
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	to, err := telemetryUnixTime(query.Get("to"), time.Now().UTC())
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	from, err := telemetryUnixTime(query.Get("from"), to.Add(-telemetryDefaultWindow))
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	step, err := telemetryParseStep(query.Get("step_s"))
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	result, err := a.Deps.Sampler.Series(name, measure, telemetrySplitList(query.Get("by")), filters, from, to, step)
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	if nil == result.Series {
		result.Series = []telemetry.SeriesPoints{}
	}
	handlers.SendJSONResponse(w, result)
}

func (a *API) Logs(w http.ResponseWriter, r *http.Request) {
	hub, ok := a.telemetryHub(w)
	if !ok {
		return
	}
	query, err := telemetryLogQuery(r)
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	page, err := hub.Logs.Query(query)
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	if nil == page.Items {
		page.Items = []telemetry.LogEntry{}
	}
	handlers.SendJSONResponse(w, page)
}

func (a *API) LogStream(w http.ResponseWriter, r *http.Request) {
	hub, ok := a.telemetryHub(w)
	if !ok {
		return
	}
	query, err := telemetryLogQuery(r)
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	sub, err := hub.Logs.Subscribe(query, telemetry.ParseLastEventID(r))
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	logger := zap.L()
	logger.Info("Log stream opened", zap.String("remote_addr", r.RemoteAddr))
	if err := telemetry.ServeSSE(w, r, sub); nil != err {
		logger.Warn("Log stream failed", zap.Error(err))
		return
	}
	logger.Info("Log stream closed")
}

func (a *API) Audit(w http.ResponseWriter, r *http.Request) {
	hub, ok := a.telemetryHub(w)
	if !ok {
		return
	}
	values := r.URL.Query()
	query, err := telemetryAuditQuery(values)
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	format := values.Get("format")
	if "" != format && "json" != format && "csv" != format {
		a.telemetryError(w, fmt.Errorf("%w: format must be json or csv", telemetry.ErrInvalidQuery))
		return
	}
	page, err := hub.Audit.Query(query)
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	if "csv" == format {
		if err := a.writeAuditCSV(w, page.Items); nil != err {
			zap.L().Warn("Audit CSV export failed", zap.Error(err))
		}
		return
	}
	if nil == page.Items {
		page.Items = []telemetry.AuditRecord{}
	}
	handlers.SendJSONResponse(w, page)
}

func (a *API) EventStream(w http.ResponseWriter, r *http.Request) {
	hub, ok := a.telemetryHub(w)
	if !ok {
		return
	}
	sub := hub.Events.Subscribe(telemetrySplitList(r.URL.Query().Get("types")), telemetry.ParseLastEventID(r))
	logger := zap.L()
	logger.Info("Event stream opened", zap.String("remote_addr", r.RemoteAddr))
	if err := telemetry.ServeSSE(w, r, sub); nil != err {
		logger.Warn("Event stream failed", zap.Error(err))
		return
	}
	logger.Info("Event stream closed")
}

func (a *API) EventHistory(w http.ResponseWriter, r *http.Request) {
	hub, ok := a.telemetryHub(w)
	if !ok {
		return
	}
	values := r.URL.Query()
	since, err := telemetryTime(values.Get("since"))
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	limit, err := telemetryLimit(values.Get("limit"))
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	events, err := hub.Events.Query(telemetry.EventQuery{
		Types: telemetrySplitList(values.Get("types")),
		Since: since,
		Limit: limit,
	})
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	if nil == events {
		events = []telemetry.Event{}
	}
	handlers.SendJSONResponse(w, struct {
		Items []telemetry.Event `json:"items"`
	}{Items: events})
}

func (a *API) Transfers(w http.ResponseWriter, r *http.Request) {
	items := []Transfer{}
	if nil != a.Deps.Transfers {
		if active := a.Deps.Transfers.ActiveTransfers(); nil != active {
			items = active
		}
	}
	handlers.SendJSONResponse(w, struct {
		Items []Transfer `json:"items"`
	}{Items: items})
}

func (a *API) telemetryHub(w http.ResponseWriter) (*telemetry.Hub, bool) {
	if nil == a.Deps.Telemetry {
		a.telemetryUnavailable(w, "telemetry")
		return nil, false
	}
	return a.Deps.Telemetry, true
}

func (a *API) telemetryUnavailable(w http.ResponseWriter, component string) {
	handlers.SendErrorResponse(w, zap.L(), fmt.Errorf("%s is not running", component), http.StatusServiceUnavailable)
}

func (a *API) telemetryError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, telemetry.ErrUnknownMetric):
		status = http.StatusNotFound
	case errors.Is(err, telemetry.ErrInvalidQuery):
		status = http.StatusBadRequest
	}
	handlers.SendErrorResponse(w, zap.L(), err, status)
}

func (a *API) writeAuditCSV(w http.ResponseWriter, items []telemetry.AuditRecord) error {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit.csv"`)
	w.WriteHeader(http.StatusOK)
	writer := csv.NewWriter(w)
	if err := writer.Write(telemetryAuditCSVHeader); nil != err {
		return err
	}
	for _, item := range items {
		row := []string{
			item.At.UTC().Format(time.RFC3339Nano),
			telemetryCSVCell(item.RequestID),
			telemetryCSVCell(item.UserID),
			telemetryCSVCell(item.Method),
			telemetryCSVCell(item.Route),
			telemetryCSVCell(item.Path),
			telemetryCSVCell(item.Op),
			strconv.Itoa(item.Status),
			strconv.FormatInt(item.BytesIn, 10),
			strconv.FormatInt(item.BytesOut, 10),
			strconv.FormatInt(item.DurationMS, 10),
			telemetryCSVCell(item.InstanceID),
			telemetryCSVCell(item.Client),
		}
		if err := writer.Write(row); nil != err {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func telemetryCSVCell(value string) string {
	if 0 < len(value) && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}

func telemetryLogQuery(r *http.Request) (telemetry.LogQuery, error) {
	values := r.URL.Query()
	since, err := telemetryTime(values.Get("since"))
	if nil != err {
		return telemetry.LogQuery{}, err
	}
	limit, err := telemetryLimit(values.Get("limit"))
	if nil != err {
		return telemetry.LogQuery{}, err
	}
	return telemetry.LogQuery{
		MinLevel:  values.Get("level"),
		Text:      values.Get("q"),
		RequestID: values.Get("request_id"),
		Since:     since,
		Limit:     limit,
		Cursor:    values.Get("cursor"),
	}, nil
}

func telemetryAuditQuery(values map[string][]string) (telemetry.AuditQuery, error) {
	since, err := telemetryTime(telemetryValue(values, "since"))
	if nil != err {
		return telemetry.AuditQuery{}, err
	}
	until, err := telemetryTime(telemetryValue(values, "until"))
	if nil != err {
		return telemetry.AuditQuery{}, err
	}
	limit, err := telemetryLimit(telemetryValue(values, "limit"))
	if nil != err {
		return telemetry.AuditQuery{}, err
	}
	return telemetry.AuditQuery{
		Since:      since,
		Until:      until,
		UserID:     telemetryValue(values, "user"),
		Op:         telemetryValue(values, "op"),
		PathPrefix: telemetryValue(values, "path"),
		Status:     telemetryValue(values, "status"),
		RequestID:  telemetryValue(values, "request_id"),
		Limit:      limit,
		Cursor:     telemetryValue(values, "cursor"),
	}, nil
}

func telemetryValue(values map[string][]string, key string) string {
	if list := values[key]; 0 < len(list) {
		return list[0]
	}
	return ""
}

func telemetrySplitList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); "" != trimmed {
			values = append(values, trimmed)
		}
	}
	return values
}

func telemetryParseFilters(raw []string) (map[string]string, error) {
	filters := make(map[string]string, len(raw))
	for _, entry := range raw {
		label, value, found := strings.Cut(entry, "=")
		label = strings.TrimSpace(label)
		value = strings.TrimSpace(value)
		if !found || "" == label || "" == value {
			return nil, fmt.Errorf("%w: filter must be label=value", telemetry.ErrInvalidQuery)
		}
		filters[label] = value
	}
	return filters, nil
}

func telemetryTime(raw string) (time.Time, error) {
	if "" == raw {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if nil != err {
		return time.Time{}, fmt.Errorf("%w: time must be RFC 3339", telemetry.ErrInvalidQuery)
	}
	return parsed.UTC(), nil
}

func telemetryUnixTime(raw string, fallback time.Time) (time.Time, error) {
	if "" == raw {
		return fallback, nil
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if nil != err {
		return time.Time{}, fmt.Errorf("%w: time must be unix seconds", telemetry.ErrInvalidQuery)
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func telemetryParseStep(raw string) (time.Duration, error) {
	if "" == raw {
		return telemetryDefaultStep, nil
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if nil != err || !(0 < seconds) || math.IsInf(seconds, 1) || telemetryMaxStepSeconds < seconds {
		return 0, fmt.Errorf("%w: step_s must be between 0 and %d", telemetry.ErrInvalidQuery, telemetryMaxStepSeconds)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func telemetryLimit(raw string) (int, error) {
	if "" == raw {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if nil != err {
		return 0, fmt.Errorf("%w: limit must be an integer", telemetry.ErrInvalidQuery)
	}
	return limit, nil
}
