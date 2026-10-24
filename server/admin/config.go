package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/server/handlers"
)

const configBodyLimit = 64 << 10

var configMu sync.Mutex

type configFieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type configKeyView struct {
	Path    string `json:"path"`
	Value   any    `json:"value"`
	Default any    `json:"default"`
	Source  string `json:"source"`
	Env     string `json:"env"`
	Secret  bool   `json:"secret"`
	Set     bool   `json:"set"`
	Restart string `json:"restart"`
	Valid   bool   `json:"valid"`
}

type saveConfigRequest struct {
	Changes          map[string]any `json:"changes"`
	ExpectedRevision string         `json:"expected_revision"`
}

type validateConfigRequest struct {
	Changes map[string]any `json:"changes"`
}

func (a *API) GetConfig(w http.ResponseWriter, r *http.Request) {
	configMu.Lock()
	defer configMu.Unlock()

	if nil == a.deps.Config {
		handlers.SendErrorResponse(w, a.configLog(), errors.New("configuration is not loaded"), http.StatusInternalServerError)
		return
	}
	running := *a.deps.Config
	data, err := a.readConfigFile()
	if nil != err {
		handlers.SendErrorResponse(w, a.configLog(), err, http.StatusInternalServerError)
		return
	}
	saved, err := a.savedConfig(data)
	if nil != err {
		handlers.SendErrorResponse(w, a.configLog(), err, http.StatusInternalServerError)
		return
	}

	runningValues := config.Flatten(running)
	var savedValues map[string]any
	if nil != saved {
		savedValues = config.Flatten(*saved)
	}

	pending := make([]string, 0)
	views := make([]configKeyView, 0, len(config.Schema()))
	for _, key := range config.Schema() {
		value := configKeyValue(runningValues, key)
		if nil != saved && !config.IsEnvOnly(key.Path) && !reflect.DeepEqual(savedValues[key.Path], runningValues[key.Path]) {
			pending = append(pending, key.Path)
		}
		view := configKeyView{
			Path:    key.Path,
			Value:   value,
			Default: key.Default,
			Source:  string(a.sourceOf(key.Path)),
			Env:     key.Env,
			Secret:  key.Secret,
			Set:     configValueSet(value),
			Restart: key.Restart,
			Valid:   true,
		}
		if key.Secret {
			view.Value = nil
		}
		views = append(views, view)
	}

	writeConfigJSON(w, http.StatusOK, map[string]any{
		"config_path":      a.deps.ConfigPath,
		"restart_required": 0 < len(pending),
		"pending":          pending,
		"revision":         config.RevisionOf(data),
		"keys":             views,
	})
}

func (a *API) ConfigSchema(w http.ResponseWriter, r *http.Request) {
	writeConfigJSON(w, http.StatusOK, map[string]any{"keys": config.Schema()})
}

func (a *API) ValidateConfig(w http.ResponseWriter, r *http.Request) {
	var request validateConfigRequest
	if decodeErr := decodeConfigBody(w, r, &request); nil != decodeErr {
		writeConfigError(w, http.StatusBadRequest, "BAD_REQUEST", decodeErr.Error(), nil)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	candidate, problems := a.candidateFor(request.Changes)
	restartPaths, appliedPaths := splitByRestart(configPaths(request.Changes))
	writeConfigJSON(w, http.StatusOK, map[string]any{
		"valid":            0 == len(problems),
		"errors":           problems,
		"warnings":         configWarnings(candidate),
		"restart_required": restartPaths,
		"applies_now":      appliedPaths,
	})
}

func (a *API) SaveConfig(w http.ResponseWriter, r *http.Request) {
	var request saveConfigRequest
	if decodeErr := decodeConfigBody(w, r, &request); nil != decodeErr {
		writeConfigError(w, http.StatusBadRequest, "BAD_REQUEST", decodeErr.Error(), nil)
		return
	}
	if 0 == len(request.Changes) {
		writeConfigError(w, http.StatusBadRequest, "BAD_REQUEST", "no settings were given to save", nil)
		return
	}
	if "" == a.deps.ConfigPath {
		writeConfigError(w, http.StatusConflict, "CONFLICT", "no config file is in use, so nothing can be saved", nil)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	logger := a.configLog()
	data, doc, err := config.ReadConfigDocument(a.deps.ConfigPath)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	revision := config.RevisionOf(data)
	if request.ExpectedRevision != revision {
		writeConfigError(w, http.StatusConflict, "CONFLICT",
			"the config file changed since it was read, reload and try again",
			map[string]any{"revision": revision})
		return
	}

	candidate, problems := a.candidateFor(request.Changes)
	if 0 < len(problems) {
		writeConfigError(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED",
			"the config does not pass validation", map[string]any{"errors": problems})
		return
	}

	for _, path := range configPaths(request.Changes) {
		if setErr := config.SetDocumentValue(doc, path, request.Changes[path]); nil != setErr {
			handlers.SendErrorResponse(w, logger, setErr, http.StatusInternalServerError)
			return
		}
	}
	backupPath, newRevision, err := config.WriteConfigDocument(a.deps.ConfigPath, data, doc, time.Now())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}

	paths := configPaths(request.Changes)
	restartPaths, appliedPaths := splitByRestart(paths)
	if applyErr := a.applyChanges(appliedPaths, &candidate); nil != applyErr {
		handlers.SendErrorResponse(w, logger, applyErr, http.StatusInternalServerError)
		return
	}

	writeConfigJSON(w, http.StatusOK, map[string]any{
		"written":          true,
		"backup_path":      backupPath,
		"revision":         newRevision,
		"restart_required": restartPaths,
		"applied_now":      appliedPaths,
	})
	logger.Info("Configuration saved",
		zap.String("config_path", a.deps.ConfigPath),
		zap.String("backup_path", backupPath),
		zap.Strings("keys", paths))
}

func (a *API) PatchRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if decodeErr := decodeConfigBody(w, r, &body); nil != decodeErr {
		writeConfigError(w, http.StatusBadRequest, "BAD_REQUEST", decodeErr.Error(), nil)
		return
	}
	persist := false
	if flag, ok := body["persist"].(bool); ok {
		persist = flag
	}
	delete(body, "persist")
	if 0 == len(body) {
		writeConfigError(w, http.StatusBadRequest, "BAD_REQUEST", "no runtime settings were given", nil)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	logger := a.configLog()
	if nil == a.deps.Config {
		handlers.SendErrorResponse(w, logger, errors.New("configuration is not loaded"), http.StatusInternalServerError)
		return
	}
	paths := configPaths(body)
	problems := make([]configFieldError, 0)
	candidate := *a.deps.Config
	for _, path := range paths {
		key, ok := config.LookupKey(path)
		if ok && "runtime" != key.Restart {
			problems = append(problems, configFieldError{Path: path, Message: "needs a restart, save it in the config file instead"})
			continue
		}
		if message := a.changeProblem(path, body[path]); "" != message {
			problems = append(problems, configFieldError{Path: path, Message: message})
			continue
		}
		if setErr := config.SetByPath(&candidate, path, body[path]); nil != setErr {
			problems = append(problems, configFieldError{Path: path, Message: setErr.Error()})
		}
	}
	if 0 == len(problems) {
		if validateErr := config.ValidateAppConfig(&candidate); nil != validateErr {
			problems = append(problems, configFieldError{Message: validateErr.Error()})
		}
	}
	if 0 < len(problems) {
		writeConfigError(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED",
			"the settings do not pass validation", map[string]any{"errors": problems})
		return
	}

	data, err := a.readConfigFile()
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	revision := config.RevisionOf(data)
	backupPath := ""
	if persist {
		if "" == a.deps.ConfigPath {
			writeConfigError(w, http.StatusConflict, "CONFLICT", "no config file is in use, so nothing can be saved", nil)
			return
		}
		fileData, doc, readErr := config.ReadConfigDocument(a.deps.ConfigPath)
		if nil != readErr {
			handlers.SendErrorResponse(w, logger, readErr, http.StatusInternalServerError)
			return
		}
		for _, path := range paths {
			if setErr := config.SetDocumentValue(doc, path, body[path]); nil != setErr {
				handlers.SendErrorResponse(w, logger, setErr, http.StatusInternalServerError)
				return
			}
		}
		var writeErr error
		backupPath, revision, writeErr = config.WriteConfigDocument(a.deps.ConfigPath, fileData, doc, time.Now())
		if nil != writeErr {
			handlers.SendErrorResponse(w, logger, writeErr, http.StatusInternalServerError)
			return
		}
	}

	if applyErr := a.applyChanges(paths, &candidate); nil != applyErr {
		handlers.SendErrorResponse(w, logger, applyErr, http.StatusInternalServerError)
		return
	}

	writeConfigJSON(w, http.StatusOK, map[string]any{
		"applied":   paths,
		"persisted": persist,
		"revision":  revision,
	})
	logger.Info("Runtime configuration applied",
		zap.Strings("keys", paths),
		zap.Bool("persisted", persist),
		zap.String("backup_path", backupPath))
}

func (a *API) ExportConfig(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if "" == format {
		format = "yaml"
	}
	effective := "false" != r.URL.Query().Get("effective")

	values, err := a.exportValues(effective)
	if nil != err {
		handlers.SendErrorResponse(w, a.configLog(), err, http.StatusInternalServerError)
		return
	}

	var body []byte
	var contentType, extension string
	switch format {
	case "yaml":
		body, err = yaml.Marshal(nestConfigValues(values))
		contentType, extension = "application/yaml", "yaml"
	case "json":
		body, err = json.MarshalIndent(nestConfigValues(values), "", "  ")
		contentType, extension = "application/json", "json"
	case "env":
		body, err = configEnvLines(values)
		contentType, extension = "text/plain; charset=utf-8", "env"
	default:
		writeConfigError(w, http.StatusBadRequest, "BAD_REQUEST", "format must be yaml, env or json", nil)
		return
	}
	if nil != err {
		handlers.SendErrorResponse(w, a.configLog(), err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="callfs-config.`+extension+`"`)
	w.WriteHeader(http.StatusOK)
	if _, writeErr := w.Write(body); nil != writeErr {
		a.configLog().Error("Failed to write config export", zap.Error(writeErr))
	}
}

func (a *API) exportValues(effective bool) (map[string]any, error) {
	configMu.Lock()
	defer configMu.Unlock()

	var runningValues map[string]any
	var fileValues map[string]any
	if effective {
		if nil == a.deps.Config {
			return nil, errors.New("configuration is not loaded")
		}
		runningValues = config.Flatten(*a.deps.Config)
	} else {
		data, err := a.readConfigFile()
		if nil != err {
			return nil, err
		}
		if unmarshalErr := yaml.Unmarshal(data, &fileValues); nil != unmarshalErr {
			return nil, fmt.Errorf("failed to parse config file: %w", unmarshalErr)
		}
	}

	values := make(map[string]any)
	for _, key := range config.Schema() {
		var value any
		ok := false
		switch {
		case config.IsEnvOnly(key.Path):
			value, ok = os.LookupEnv(key.Env)
		case effective:
			value, ok = runningValues[key.Path]
		default:
			value, ok = config.LookupNested(fileValues, key.Path)
		}
		if !ok {
			continue
		}
		if key.Secret {
			value = redactedSecret(value)
		}
		values[key.Path] = value
	}
	return values, nil
}

func redactedSecret(value any) string {
	if configValueSet(value) {
		return "<set>"
	}
	return ""
}

func nestConfigValues(values map[string]any) map[string]any {
	root := make(map[string]any)
	for path, value := range values {
		segments := strings.Split(path, ".")
		section := root
		for _, segment := range segments[:len(segments)-1] {
			next, ok := section[segment].(map[string]any)
			if !ok {
				next = make(map[string]any)
				section[segment] = next
			}
			section = next
		}
		section[segments[len(segments)-1]] = value
	}
	return root
}

func configEnvLines(values map[string]any) ([]byte, error) {
	var builder strings.Builder
	for _, key := range config.Schema() {
		value, ok := values[key.Path]
		if !ok {
			continue
		}
		text, isText := value.(string)
		if !isText {
			encoded, err := json.Marshal(value)
			if nil != err {
				return nil, fmt.Errorf("failed to encode %s: %w", key.Path, err)
			}
			text = string(encoded)
		}
		builder.WriteString(key.Env + "=" + text + "\n")
	}
	return []byte(builder.String()), nil
}

func (a *API) readConfigFile() ([]byte, error) {
	if "" == a.deps.ConfigPath {
		return nil, nil
	}
	data, err := os.ReadFile(a.deps.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if nil != err {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return data, nil
}

func (a *API) savedConfig(data []byte) (*config.AppConfig, error) {
	if 0 == len(data) {
		return nil, nil
	}
	loaded, err := config.LoadConfigFromFile(a.deps.ConfigPath)
	if nil != err {
		return nil, err
	}
	return &loaded, nil
}

func (a *API) baseConfig() (config.AppConfig, error) {
	if "" == a.deps.ConfigPath {
		return config.LoadConfig()
	}
	data, err := a.readConfigFile()
	if nil != err {
		return config.AppConfig{}, err
	}
	if 0 == len(data) {
		return config.DefaultAppConfig(), nil
	}
	return config.LoadConfigFromFile(a.deps.ConfigPath)
}

func (a *API) candidateFor(changes map[string]any) (config.AppConfig, []configFieldError) {
	problems := make([]configFieldError, 0)
	base, err := a.baseConfig()
	if nil != err {
		return base, append(problems, configFieldError{Message: err.Error()})
	}
	for _, path := range configPaths(changes) {
		if message := a.changeProblem(path, changes[path]); "" != message {
			problems = append(problems, configFieldError{Path: path, Message: message})
			continue
		}
		if setErr := config.SetByPath(&base, path, changes[path]); nil != setErr {
			problems = append(problems, configFieldError{Path: path, Message: setErr.Error()})
		}
	}
	if 0 == len(problems) {
		if validateErr := config.ValidateAppConfig(&base); nil != validateErr {
			problems = append(problems, configFieldError{Message: validateErr.Error()})
		}
	}
	return base, problems
}

func (a *API) changeProblem(path string, value any) string {
	if _, ok := config.LookupKey(path); !ok {
		return "unknown setting"
	}
	if config.IsEnvOnly(path) || config.SourceEnv == a.sourceOf(path) {
		return "set by environment"
	}
	if checkErr := config.CheckValue(path, value); nil != checkErr {
		return checkErr.Error()
	}
	return ""
}

func (a *API) applyChanges(paths []string, candidate *config.AppConfig) error {
	if 0 == len(paths) {
		return nil
	}
	if nil == a.deps.Config {
		return errors.New("configuration is not loaded")
	}
	for _, path := range paths {
		if copyErr := config.CopyByPath(a.deps.Config, candidate, path); nil != copyErr {
			return copyErr
		}
		a.setSource(path, config.SourceRuntime)
	}
	return a.applyLive(paths)
}

func (a *API) applyLive(paths []string) error {
	cfg := a.deps.Config
	reloadAuth := false
	reloadLimits := false
	for _, path := range paths {
		switch path {
		case "server.metadata_cache_ttl":
			if nil != a.deps.Engine {
				a.deps.Engine.SetMetadataCacheTTL(cfg.Server.MetadataCacheTTL)
			}
		case "dlm.wait_timeout":
			if setter, ok := a.deps.Locks.(interface{ SetWaitTimeout(time.Duration) }); ok {
				setter.SetWaitTimeout(cfg.DLM.WaitTimeout)
			}
		case "auth.api_keys", "auth.api_key_users", "auth.admin_users":
			reloadAuth = true
		}
		if strings.HasPrefix(path, "server.rate_limit.") {
			reloadLimits = true
		}
	}
	if reloadAuth {
		if nil != a.deps.Authenticator {
			a.deps.Authenticator.Reload(cfg.Auth.APIKeys, cfg.Auth.APIKeyUsers, cfg.Auth.InternalProxySecret)
		}
		if nil != a.deps.Admins {
			a.deps.Admins.Replace(cfg.Auth.AdminUsers)
		}
	}
	if reloadLimits && nil != a.deps.RateLimits {
		limits := []struct {
			endpoint string
			rps      float64
			burst    int
		}{
			{"download", cfg.Server.RateLimit.DownloadRPS, cfg.Server.RateLimit.DownloadBurst},
			{"link_generate", cfg.Server.RateLimit.LinkGenerateRPS, cfg.Server.RateLimit.LinkGenerateBurst},
			{"session", cfg.Server.RateLimit.SessionRPS, cfg.Server.RateLimit.SessionBurst},
		}
		for _, limit := range limits {
			if setErr := a.deps.RateLimits.SetLimits(limit.endpoint, limit.rps, limit.burst); nil != setErr {
				return setErr
			}
		}
	}
	return nil
}

func (a *API) sourceOf(path string) config.Source {
	if source, ok := a.deps.ConfigSources[path]; ok {
		return source
	}
	return config.SourceDefault
}

func (a *API) setSource(path string, source config.Source) {
	if nil == a.deps.ConfigSources {
		return
	}
	if config.SourceEnv == a.deps.ConfigSources[path] {
		return
	}
	a.deps.ConfigSources[path] = source
}

func (a *API) configLog() *zap.Logger {
	if nil == a.deps.Logger {
		return zap.NewNop()
	}
	return a.deps.Logger
}

func configKeyValue(flat map[string]any, key config.SchemaKey) any {
	if config.IsEnvOnly(key.Path) {
		if value, ok := os.LookupEnv(key.Env); ok {
			return value
		}
		return key.Default
	}
	return flat[key.Path]
}

func configValueSet(value any) bool {
	if nil == value {
		return false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.String, reflect.Slice, reflect.Map:
		return 0 < reflected.Len()
	default:
		return !reflected.IsZero()
	}
}

func configPaths(changes map[string]any) []string {
	paths := make([]string, 0, len(changes))
	for path := range changes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func splitByRestart(paths []string) ([]string, []string) {
	restart := make([]string, 0)
	applied := make([]string, 0)
	for _, path := range paths {
		key, ok := config.LookupKey(path)
		if ok && "restart" != key.Restart {
			applied = append(applied, path)
			continue
		}
		restart = append(restart, path)
	}
	return restart, applied
}

func configWarnings(cfg config.AppConfig) []configFieldError {
	warnings := make([]configFieldError, 0)
	if "http" == strings.ToLower(cfg.Server.Protocol) {
		warnings = append(warnings, configFieldError{Path: "server.protocol", Message: "Access keys travel without encryption."})
	}
	if 32 > len(cfg.Auth.InternalProxySecret) {
		warnings = append(warnings, configFieldError{Path: "auth.internal_proxy_secret", Message: "Shared server secret is shorter than 32 characters."})
	}
	return warnings
}

func decodeConfigBody(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, configBodyLimit))
	if decodeErr := decoder.Decode(target); nil != decodeErr {
		return errors.New("request body must be a JSON object of at most 64 KiB")
	}
	return nil
}

func writeConfigJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	handlers.SendJSONResponse(w, body)
}

func writeConfigError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeConfigJSON(w, status, handlers.ErrorResponse{Code: code, Message: message, Details: details})
}
