package sonalmod

import "github.com/gemyago/sonalmod/apps/sonalmod/internal"

// EngineOpt allows configuring the engine configuration.
type EngineOpt = internal.EngineOpt

// WithEngineLogsFormatJSON sets the logs format to JSON.
func WithEngineLogsFormatJSON(logsFormatJSON bool) EngineOpt { //nolint:ireturn
	return internal.EngineCfgOptFunc(func(opts *internal.EngineCfg) {
		opts.LogsFormatJSON = logsFormatJSON
	})
}

// WithEngineLogsOutputFile sets the logs output file.
func WithEngineLogsOutputFile(logsOutputFile string) EngineOpt { //nolint:ireturn
	return internal.EngineCfgOptFunc(func(opts *internal.EngineCfg) {
		opts.LogsOutputFile = logsOutputFile
	})
}

// WithEngineDefaultLogLevel sets the default log level.
// Value must be [slog.Level] string representation.
func WithEngineDefaultLogLevel(defaultLogLevel string) EngineOpt { //nolint:ireturn
	return internal.EngineCfgOptFunc(func(opts *internal.EngineCfg) {
		if defaultLogLevel == "" {
			return
		}
		opts.DefaultLogLevel = &defaultLogLevel
	})
}

// WithEngineEnv sets which embedded config layer loads (e.g. test, local).
// Empty string leaves default behavior in [config.Load].
// TODO: Make this internal
func WithEngineEnv(env string) EngineOpt { //nolint:ireturn
	return internal.EngineCfgOptFunc(func(opts *internal.EngineCfg) {
		opts.Env = env
	})
}

type engineStartServerCfg struct {
	noop       bool
	uiLocation string
}

// EngineStartServerOpt allows configuring the start server operation.
type EngineStartServerOpt interface {
	apply(opts *engineStartServerCfg)
}

type engineStartServerOptFunc func(opts *engineStartServerCfg)

func (f engineStartServerOptFunc) apply(opts *engineStartServerCfg) {
	f(opts)
}

// WithStartHTTPServerNoop sets the noop flag for the HTTP server.
// Useful for testing if dependencies are setup correctly.
func WithStartHTTPServerNoop(noop bool) EngineStartServerOpt { //nolint:ireturn
	return engineStartServerOptFunc(func(opts *engineStartServerCfg) {
		opts.noop = noop
	})
}

// WithStartHTTPServerUILocation sets the UI location for the HTTP server.
// Useful for serving pre-built UI static assets.
func WithStartHTTPServerUILocation(uiLocation string) EngineStartServerOpt { //nolint:ireturn
	return engineStartServerOptFunc(func(opts *engineStartServerCfg) {
		opts.uiLocation = uiLocation
	})
}
