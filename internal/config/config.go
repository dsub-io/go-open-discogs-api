package config

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ApplicationName = "go-open-discogs-api"

	FlagAddress           = "address"
	FlagManagementAddress = "management-address"
	FlagServerURL         = "server-url"
	FlagCacheControl      = "cache-control"
	FlagAccessLog         = "access-log"
	FlagMaxProcs          = "max-procs"
	FlagMemoryLimitMiB    = "memory-limit-mib"
	FlagLogLevel          = "log-level"
	FlagQueryTimeout      = "query-timeout"
	FlagShutdownTimeout   = "shutdown-timeout"
	FlagDatabaseURL       = "database-url"
	FlagDatabaseSchema    = "database-schema"
	FlagDatabaseHost      = "db-host"
	FlagDatabaseUsername  = "db-username"
	FlagDatabasePassword  = "db-password"
	FlagDatabaseName      = "db-database"
	FlagDatabaseSSLMode   = "db-sslmode"
	FlagDatabaseMaxConns  = "db-max-conns"
	FlagDatabaseMinConns  = "db-min-conns"
	FlagStatementCache    = "db-statement-cache"
	FlagMetricsEnabled    = "metrics-enabled"
	FlagTracingEnabled    = "tracing-enabled"
	FlagOTLPEndpoint      = "otlp-endpoint"
	FlagTraceSampleRatio  = "trace-sample-ratio"
	FlagHealthcheck       = "healthcheck"
	FlagVersion           = "version"

	EnvAddress           = "API_ADDRESS"
	EnvManagementAddress = "API_MANAGEMENT_ADDRESS"
	EnvServerURL         = "API_SERVER_URL"
	EnvCacheControl      = "API_CACHE_CONTROL"
	EnvAccessLog         = "API_ACCESS_LOG"
	EnvMaxProcs          = "API_MAX_PROCS"
	EnvMemoryLimitMiB    = "API_MEMORY_LIMIT_MIB"
	EnvLogLevel          = "API_LOG_LEVEL"
	EnvQueryTimeout      = "API_QUERY_TIMEOUT"
	EnvShutdownTimeout   = "API_SHUTDOWN_TIMEOUT"
	EnvDatabaseURL       = "API_DATABASE_URL"
	EnvDatabaseSchema    = "API_DATABASE_SCHEMA"
	EnvDatabaseHost      = "API_DB_HOST"
	EnvDatabaseUsername  = "API_DB_USERNAME"
	EnvDatabasePassword  = "API_DB_PASSWORD"
	EnvDatabaseName      = "API_DB_DATABASE"
	EnvDatabaseSSLMode   = "API_DB_SSLMODE"
	EnvDatabaseMaxConns  = "API_DB_MAX_CONNS"
	EnvDatabaseMinConns  = "API_DB_MIN_CONNS"
	EnvStatementCache    = "API_DB_STATEMENT_CACHE"
	EnvMetricsEnabled    = "API_METRICS_ENABLED"
	EnvTracingEnabled    = "API_TRACING_ENABLED"
	EnvOTLPEndpoint      = "OTEL_EXPORTER_OTLP_ENDPOINT"
	EnvTraceSampleRatio  = "OTEL_TRACES_SAMPLER_ARG"

	DefaultAddress            = ":8080"
	DefaultManagementAddress  = "127.0.0.1:8081"
	DefaultServerURL          = "http://localhost:8080"
	DefaultCacheControl       = "public, max-age=60, stale-while-revalidate=300"
	DefaultLogLevel           = "info"
	DefaultDatabaseName       = "discogs"
	DefaultDatabaseSchema     = "public"
	DefaultDatabaseSSLMode    = "prefer"
	DefaultDatabaseMaxConns   = 10
	DefaultDatabaseMinConns   = 0
	DefaultStatementCacheSize = 128
	DefaultTraceSampleRatio   = 0.1
	DefaultReadHeaderTimeout  = 5 * time.Second
	DefaultReadTimeout        = 15 * time.Second
	DefaultWriteTimeout       = 30 * time.Second
	DefaultIdleTimeout        = 90 * time.Second
	DefaultShutdownTimeout    = 30 * time.Second
	DefaultQueryTimeout       = 10 * time.Second
	DefaultMaxConnectionIdle  = 5 * time.Minute
	DefaultMaxConnectionLife  = 30 * time.Minute
	DefaultHealthCheckPeriod  = 30 * time.Second
	MaximumConnections        = 1024
	MaximumStatementCacheSize = 65536
	MaximumSchemaNameLength   = 63

	TypeString   ValueType = "string"
	TypeBoolean  ValueType = "boolean"
	TypeInteger  ValueType = "integer"
	TypeDuration ValueType = "duration"
	TypeFloat    ValueType = "float"

	RequirementOptional    Requirement = "optional"
	RequirementConditional Requirement = "conditional"
)

type LookupEnv func(string) (string, bool)

type ValueType string

type Requirement string

type Definition struct {
	Flag        string
	Environment string
	Type        ValueType
	Default     string
	Requirement Requirement
	Sensitive   bool
	Description string
}

type Result struct {
	Config         Config
	CheckReadiness bool
	ShowVersion    bool
}

type Config struct {
	PublicAddress     string
	ManagementAddress string
	ServerURL         string
	CacheControl      string
	AccessLog         bool
	MaxProcs          int
	MemoryLimitBytes  int64
	LogLevel          slog.Level
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	QueryTimeout      time.Duration
	MetricsEnabled    bool
	Tracing           Tracing
	Database          Database
}

type Tracing struct {
	Enabled     bool
	Endpoint    string
	SampleRatio float64
}

type Database struct {
	URL                string
	Schema             string
	MaxConnections     int32
	MinConnections     int32
	MaxConnectionIdle  time.Duration
	MaxConnectionLife  time.Duration
	HealthCheckPeriod  time.Duration
	StatementCacheSize int
}

type rawValues struct {
	address           string
	managementAddress string
	serverURL         string
	cacheControl      string
	accessLog         bool
	maxProcs          int
	memoryLimitMiB    int
	logLevel          string
	queryTimeout      time.Duration
	shutdownTimeout   time.Duration
	databaseURL       string
	databaseSchema    string
	databaseHost      string
	databaseUsername  string
	databasePassword  string
	databaseName      string
	databaseSSLMode   string
	databaseMaxConns  int64
	databaseMinConns  int64
	statementCache    int
	metricsEnabled    bool
	tracingEnabled    bool
	otlpEndpoint      string
	traceSampleRatio  float64
}

func Load(arguments []string, lookup LookupEnv, output io.Writer) (Result, error) {
	if hasArgument(arguments, "--"+FlagVersion, "-"+FlagVersion) {
		return Result{ShowVersion: true}, nil
	}
	if hasArgument(arguments, "--"+FlagHealthcheck, "-"+FlagHealthcheck) {
		return Result{CheckReadiness: true}, nil
	}
	if hasArgument(arguments, "--help", "-help", "-h") {
		values, _ := environmentValues(emptyLookup)
		flags := flag.NewFlagSet(ApplicationName, flag.ContinueOnError)
		flags.SetOutput(output)
		bindFlags(flags, &values)
		return Result{}, flags.Parse(arguments)
	}
	values, err := environmentValues(lookup)
	if err != nil {
		return Result{}, err
	}
	flags := flag.NewFlagSet(ApplicationName, flag.ContinueOnError)
	flags.SetOutput(output)
	bindFlags(flags, &values)
	if err := flags.Parse(arguments); err != nil {
		return Result{}, err
	}
	if flags.NArg() != 0 {
		return Result{}, fmt.Errorf("unexpected positional argument %q", flags.Arg(0))
	}
	config, err := validate(values)
	if err != nil {
		return Result{}, err
	}
	return Result{Config: config}, nil
}

func Inventory() []Definition {
	definitions := []Definition{
		{Flag: FlagAddress, Environment: EnvAddress, Type: TypeString, Default: DefaultAddress, Requirement: RequirementOptional, Description: "Public HTTP listener."},
		{Flag: FlagManagementAddress, Environment: EnvManagementAddress, Type: TypeString, Default: DefaultManagementAddress, Requirement: RequirementOptional, Description: "Management HTTP listener."},
		{Flag: FlagServerURL, Environment: EnvServerURL, Type: TypeString, Default: DefaultServerURL, Requirement: RequirementOptional, Description: "Public URL used in API links."},
		{Flag: FlagCacheControl, Environment: EnvCacheControl, Type: TypeString, Default: DefaultCacheControl, Requirement: RequirementOptional, Description: "Cache-Control value for successful API responses."},
		{Flag: FlagAccessLog, Environment: EnvAccessLog, Type: TypeBoolean, Default: "false", Requirement: RequirementOptional, Description: "Emit one structured log per HTTP request."},
		{Flag: FlagMaxProcs, Environment: EnvMaxProcs, Type: TypeInteger, Default: "0", Requirement: RequirementOptional, Description: "Exact GOMAXPROCS when positive; zero applies no application limit."},
		{Flag: FlagMemoryLimitMiB, Environment: EnvMemoryLimitMiB, Type: TypeInteger, Default: "0", Requirement: RequirementOptional, Description: "Go soft memory limit in MiB when positive; zero applies no application limit."},
		{Flag: FlagLogLevel, Environment: EnvLogLevel, Type: TypeString, Default: DefaultLogLevel, Requirement: RequirementOptional, Description: "Structured log level."},
		{Flag: FlagQueryTimeout, Environment: EnvQueryTimeout, Type: TypeDuration, Default: DefaultQueryTimeout.String(), Requirement: RequirementOptional, Description: "Maximum PostgreSQL operation duration."},
		{Flag: FlagShutdownTimeout, Environment: EnvShutdownTimeout, Type: TypeDuration, Default: DefaultShutdownTimeout.String(), Requirement: RequirementOptional, Description: "Graceful shutdown deadline."},
		{Flag: FlagDatabaseURL, Environment: EnvDatabaseURL, Type: TypeString, Requirement: RequirementConditional, Sensitive: true, Description: "Complete PostgreSQL URL; overrides split database settings."},
		{Flag: FlagDatabaseSchema, Environment: EnvDatabaseSchema, Type: TypeString, Default: DefaultDatabaseSchema, Requirement: RequirementOptional, Description: "PostgreSQL schema containing canonical OpenDiscogs tables."},
		{Flag: FlagDatabaseHost, Environment: EnvDatabaseHost, Type: TypeString, Requirement: RequirementConditional, Description: "PostgreSQL host and port when database-url is unset."},
		{Flag: FlagDatabaseUsername, Environment: EnvDatabaseUsername, Type: TypeString, Requirement: RequirementConditional, Description: "PostgreSQL username when database-url is unset."},
		{Flag: FlagDatabasePassword, Environment: EnvDatabasePassword, Type: TypeString, Requirement: RequirementConditional, Sensitive: true, Description: "PostgreSQL password when database-url is unset."},
		{Flag: FlagDatabaseName, Environment: EnvDatabaseName, Type: TypeString, Default: DefaultDatabaseName, Requirement: RequirementOptional, Description: "PostgreSQL database name."},
		{Flag: FlagDatabaseSSLMode, Environment: EnvDatabaseSSLMode, Type: TypeString, Default: DefaultDatabaseSSLMode, Requirement: RequirementOptional, Description: "PostgreSQL sslmode."},
		{Flag: FlagDatabaseMaxConns, Environment: EnvDatabaseMaxConns, Type: TypeInteger, Default: strconv.Itoa(DefaultDatabaseMaxConns), Requirement: RequirementOptional, Description: "Maximum PostgreSQL connections."},
		{Flag: FlagDatabaseMinConns, Environment: EnvDatabaseMinConns, Type: TypeInteger, Default: strconv.Itoa(DefaultDatabaseMinConns), Requirement: RequirementOptional, Description: "Minimum PostgreSQL connections."},
		{Flag: FlagStatementCache, Environment: EnvStatementCache, Type: TypeInteger, Default: strconv.Itoa(DefaultStatementCacheSize), Requirement: RequirementOptional, Description: "Per-connection prepared statement cache size."},
		{Flag: FlagMetricsEnabled, Environment: EnvMetricsEnabled, Type: TypeBoolean, Default: "true", Requirement: RequirementOptional, Description: "Expose local Prometheus metrics on the management listener."},
		{Flag: FlagTracingEnabled, Environment: EnvTracingEnabled, Type: TypeBoolean, Default: "false", Requirement: RequirementOptional, Description: "Enable OTLP trace export."},
		{Flag: FlagOTLPEndpoint, Environment: EnvOTLPEndpoint, Type: TypeString, Requirement: RequirementConditional, Description: "OTLP HTTP endpoint required only when tracing is enabled."},
		{Flag: FlagTraceSampleRatio, Environment: EnvTraceSampleRatio, Type: TypeFloat, Default: strconv.FormatFloat(DefaultTraceSampleRatio, 'f', -1, 64), Requirement: RequirementOptional, Description: "Parent-based trace sampling ratio."},
	}
	return append([]Definition(nil), definitions...)
}

func bindFlags(flags *flag.FlagSet, values *rawValues) {
	flags.StringVar(&values.address, FlagAddress, values.address, description(FlagAddress))
	flags.StringVar(&values.managementAddress, FlagManagementAddress, values.managementAddress, description(FlagManagementAddress))
	flags.StringVar(&values.serverURL, FlagServerURL, values.serverURL, description(FlagServerURL))
	flags.StringVar(&values.cacheControl, FlagCacheControl, values.cacheControl, description(FlagCacheControl))
	flags.BoolVar(&values.accessLog, FlagAccessLog, values.accessLog, description(FlagAccessLog))
	flags.IntVar(&values.maxProcs, FlagMaxProcs, values.maxProcs, description(FlagMaxProcs))
	flags.IntVar(&values.memoryLimitMiB, FlagMemoryLimitMiB, values.memoryLimitMiB, description(FlagMemoryLimitMiB))
	flags.StringVar(&values.logLevel, FlagLogLevel, values.logLevel, description(FlagLogLevel))
	flags.DurationVar(&values.queryTimeout, FlagQueryTimeout, values.queryTimeout, description(FlagQueryTimeout))
	flags.DurationVar(&values.shutdownTimeout, FlagShutdownTimeout, values.shutdownTimeout, description(FlagShutdownTimeout))
	flags.StringVar(&values.databaseURL, FlagDatabaseURL, values.databaseURL, description(FlagDatabaseURL))
	flags.StringVar(&values.databaseSchema, FlagDatabaseSchema, values.databaseSchema, description(FlagDatabaseSchema))
	flags.StringVar(&values.databaseHost, FlagDatabaseHost, values.databaseHost, description(FlagDatabaseHost))
	flags.StringVar(&values.databaseUsername, FlagDatabaseUsername, values.databaseUsername, description(FlagDatabaseUsername))
	flags.StringVar(&values.databasePassword, FlagDatabasePassword, values.databasePassword, description(FlagDatabasePassword))
	flags.StringVar(&values.databaseName, FlagDatabaseName, values.databaseName, description(FlagDatabaseName))
	flags.StringVar(&values.databaseSSLMode, FlagDatabaseSSLMode, values.databaseSSLMode, description(FlagDatabaseSSLMode))
	flags.Int64Var(&values.databaseMaxConns, FlagDatabaseMaxConns, values.databaseMaxConns, description(FlagDatabaseMaxConns))
	flags.Int64Var(&values.databaseMinConns, FlagDatabaseMinConns, values.databaseMinConns, description(FlagDatabaseMinConns))
	flags.IntVar(&values.statementCache, FlagStatementCache, values.statementCache, description(FlagStatementCache))
	flags.BoolVar(&values.metricsEnabled, FlagMetricsEnabled, values.metricsEnabled, description(FlagMetricsEnabled))
	flags.BoolVar(&values.tracingEnabled, FlagTracingEnabled, values.tracingEnabled, description(FlagTracingEnabled))
	flags.StringVar(&values.otlpEndpoint, FlagOTLPEndpoint, values.otlpEndpoint, description(FlagOTLPEndpoint))
	flags.Float64Var(&values.traceSampleRatio, FlagTraceSampleRatio, values.traceSampleRatio, description(FlagTraceSampleRatio))
	flags.Bool(FlagHealthcheck, false, "Probe management readiness and exit.")
	flags.Bool(FlagVersion, false, "Print version and exit.")
}

func environmentValues(lookup LookupEnv) (rawValues, error) {
	values := rawValues{
		address: value(lookup, EnvAddress, DefaultAddress), managementAddress: value(lookup, EnvManagementAddress, DefaultManagementAddress),
		serverURL: value(lookup, EnvServerURL, DefaultServerURL), cacheControl: value(lookup, EnvCacheControl, DefaultCacheControl),
		logLevel: value(lookup, EnvLogLevel, DefaultLogLevel), queryTimeout: DefaultQueryTimeout,
		shutdownTimeout: DefaultShutdownTimeout, databaseURL: value(lookup, EnvDatabaseURL, ""),
		databaseSchema: value(lookup, EnvDatabaseSchema, DefaultDatabaseSchema),
		databaseHost:   value(lookup, EnvDatabaseHost, ""), databaseUsername: value(lookup, EnvDatabaseUsername, ""),
		databasePassword: value(lookup, EnvDatabasePassword, ""), databaseName: value(lookup, EnvDatabaseName, DefaultDatabaseName),
		databaseSSLMode: value(lookup, EnvDatabaseSSLMode, DefaultDatabaseSSLMode), databaseMaxConns: DefaultDatabaseMaxConns,
		databaseMinConns: DefaultDatabaseMinConns, statementCache: DefaultStatementCacheSize,
		metricsEnabled: true, otlpEndpoint: value(lookup, EnvOTLPEndpoint, ""), traceSampleRatio: DefaultTraceSampleRatio,
	}
	var err error
	if values.accessLog, err = environmentBool(lookup, EnvAccessLog, false); err != nil {
		return rawValues{}, err
	}
	if values.maxProcs, err = environmentInt(lookup, EnvMaxProcs, 0); err != nil {
		return rawValues{}, err
	}
	if values.memoryLimitMiB, err = environmentInt(lookup, EnvMemoryLimitMiB, 0); err != nil {
		return rawValues{}, err
	}
	if values.queryTimeout, err = environmentDuration(lookup, EnvQueryTimeout, DefaultQueryTimeout); err != nil {
		return rawValues{}, err
	}
	if values.shutdownTimeout, err = environmentDuration(lookup, EnvShutdownTimeout, DefaultShutdownTimeout); err != nil {
		return rawValues{}, err
	}
	if values.databaseMaxConns, err = environmentInt64(lookup, EnvDatabaseMaxConns, DefaultDatabaseMaxConns); err != nil {
		return rawValues{}, err
	}
	if values.databaseMinConns, err = environmentInt64(lookup, EnvDatabaseMinConns, DefaultDatabaseMinConns); err != nil {
		return rawValues{}, err
	}
	if values.statementCache, err = environmentInt(lookup, EnvStatementCache, DefaultStatementCacheSize); err != nil {
		return rawValues{}, err
	}
	if values.metricsEnabled, err = environmentBool(lookup, EnvMetricsEnabled, true); err != nil {
		return rawValues{}, err
	}
	if values.tracingEnabled, err = environmentBool(lookup, EnvTracingEnabled, false); err != nil {
		return rawValues{}, err
	}
	if values.traceSampleRatio, err = environmentFloat(lookup, EnvTraceSampleRatio, DefaultTraceSampleRatio); err != nil {
		return rawValues{}, err
	}
	return values, nil
}

func validate(values rawValues) (Config, error) {
	serverURL, err := absoluteHTTPURL(values.serverURL)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", FlagServerURL, err)
	}
	if err := validateAddress(FlagAddress, values.address); err != nil {
		return Config{}, err
	}
	if err := validateAddress(FlagManagementAddress, values.managementAddress); err != nil {
		return Config{}, err
	}
	level, err := logLevel(values.logLevel)
	if err != nil {
		return Config{}, err
	}
	databaseURL, err := buildDatabaseURL(values)
	if err != nil {
		return Config{}, err
	}
	if values.maxProcs < 0 {
		return Config{}, fmt.Errorf("%s must be zero or positive", FlagMaxProcs)
	}
	if !validSchemaName(values.databaseSchema) {
		return Config{}, fmt.Errorf(
			"%s must be 1 to %d lowercase letters, digits, or underscores and start with a letter or underscore",
			FlagDatabaseSchema,
			MaximumSchemaNameLength,
		)
	}
	if values.memoryLimitMiB < 0 {
		return Config{}, fmt.Errorf("%s must be zero or positive", FlagMemoryLimitMiB)
	}
	if int64(values.memoryLimitMiB) > math.MaxInt64>>20 {
		return Config{}, fmt.Errorf("%s is too large to represent in bytes", FlagMemoryLimitMiB)
	}
	if values.databaseMaxConns > math.MaxInt32 || values.databaseMinConns > math.MaxInt32 {
		return Config{}, fmt.Errorf(
			"%s and %s must fit signed 32-bit PostgreSQL connection counts",
			FlagDatabaseMaxConns,
			FlagDatabaseMinConns,
		)
	}
	if values.databaseMaxConns < 1 || values.databaseMaxConns > MaximumConnections {
		return Config{}, fmt.Errorf("%s must be between 1 and %d", FlagDatabaseMaxConns, MaximumConnections)
	}
	if values.databaseMinConns < 0 || values.databaseMinConns > values.databaseMaxConns {
		return Config{}, fmt.Errorf("%s must be between 0 and %s", FlagDatabaseMinConns, FlagDatabaseMaxConns)
	}
	if values.statementCache < 0 || values.statementCache > MaximumStatementCacheSize {
		return Config{}, fmt.Errorf("%s must be between 0 and %d", FlagStatementCache, MaximumStatementCacheSize)
	}
	if values.queryTimeout <= 0 || values.shutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("timeouts must be positive")
	}
	if values.traceSampleRatio < 0 || values.traceSampleRatio > 1 {
		return Config{}, fmt.Errorf("%s must be between 0 and 1", FlagTraceSampleRatio)
	}
	if values.tracingEnabled {
		if _, err := absoluteHTTPURL(values.otlpEndpoint); err != nil {
			return Config{}, fmt.Errorf("%s is required when tracing is enabled: %w", FlagOTLPEndpoint, err)
		}
	}
	return Config{
		PublicAddress: values.address, ManagementAddress: values.managementAddress,
		ServerURL: serverURL, CacheControl: values.cacheControl, AccessLog: values.accessLog,
		MaxProcs: values.maxProcs, MemoryLimitBytes: int64(values.memoryLimitMiB) << 20,
		LogLevel: level, ReadHeaderTimeout: DefaultReadHeaderTimeout,
		ReadTimeout: DefaultReadTimeout, WriteTimeout: DefaultWriteTimeout, IdleTimeout: DefaultIdleTimeout,
		ShutdownTimeout: values.shutdownTimeout, QueryTimeout: values.queryTimeout, MetricsEnabled: values.metricsEnabled,
		Tracing: Tracing{Enabled: values.tracingEnabled, Endpoint: values.otlpEndpoint, SampleRatio: values.traceSampleRatio},
		Database: Database{
			URL: databaseURL, Schema: values.databaseSchema,
			MaxConnections: int32(values.databaseMaxConns), MinConnections: int32(values.databaseMinConns),
			MaxConnectionIdle: DefaultMaxConnectionIdle, MaxConnectionLife: DefaultMaxConnectionLife,
			HealthCheckPeriod: DefaultHealthCheckPeriod, StatementCacheSize: values.statementCache,
		},
	}, nil
}

var schemaNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func validSchemaName(name string) bool {
	return len(name) > 0 && len(name) <= MaximumSchemaNameLength && schemaNamePattern.MatchString(name)
}

func buildDatabaseURL(values rawValues) (string, error) {
	if raw := strings.TrimSpace(values.databaseURL); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
			return "", fmt.Errorf("%s must be an absolute PostgreSQL URL", FlagDatabaseURL)
		}
		return raw, nil
	}
	host := strings.TrimSpace(values.databaseHost)
	username := strings.TrimSpace(values.databaseUsername)
	database := strings.TrimSpace(values.databaseName)
	if host == "" || username == "" || values.databasePassword == "" || database == "" {
		return "", fmt.Errorf("set %s or the split database settings", FlagDatabaseURL)
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		return "", fmt.Errorf("%s must include host and port: %w", FlagDatabaseHost, err)
	}
	if strings.Contains(database, "/") {
		return "", fmt.Errorf("%s must be a database name", FlagDatabaseName)
	}
	parsed := &url.URL{Scheme: "postgres", Host: host, Path: database, User: url.UserPassword(username, values.databasePassword)}
	query := parsed.Query()
	query.Set("sslmode", strings.TrimSpace(values.databaseSSLMode))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func description(flagName string) string {
	for _, definition := range Inventory() {
		if definition.Flag == flagName {
			return definition.Description + " ENV: " + definition.Environment + "."
		}
	}
	return flagName
}

func value(lookup LookupEnv, name, fallback string) string {
	if configured, ok := lookup(name); ok {
		return configured
	}
	return fallback
}

func environmentBool(lookup LookupEnv, name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(value(lookup, name, ""))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return parsed, nil
}

func environmentInt(lookup LookupEnv, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(value(lookup, name, ""))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
}

func environmentInt64(lookup LookupEnv, name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(value(lookup, name, ""))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
}

func environmentFloat(lookup LookupEnv, name string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(value(lookup, name, ""))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a float", name)
	}
	return parsed, nil
}

func environmentDuration(lookup LookupEnv, name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(value(lookup, name, ""))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration", name)
	}
	return parsed, nil
}

func absoluteHTTPURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("must be an absolute HTTP(S) URL")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateAddress(name, address string) error {
	if _, err := net.ResolveTCPAddr("tcp", address); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func logLevel(raw string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(strings.TrimSpace(raw)))); err != nil {
		return 0, fmt.Errorf("%s must be debug, info, warn, or error", FlagLogLevel)
	}
	return level, nil
}

func hasArgument(arguments []string, candidates ...string) bool {
	for _, argument := range arguments {
		for _, candidate := range candidates {
			if argument == candidate {
				return true
			}
		}
	}
	return false
}

func emptyLookup(string) (string, bool) {
	return "", false
}
