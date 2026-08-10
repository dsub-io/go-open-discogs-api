package config

import (
	"bytes"
	"errors"
	"flag"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

const testDatabaseURL = "postgres://reader:secret@localhost:5432/discogs?sslmode=disable"

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Parallel()
	lookup := mapLookup(map[string]string{EnvDatabaseURL: testDatabaseURL})
	result, err := Load(nil, lookup, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	config := result.Config
	if config.PublicAddress != DefaultAddress || config.ManagementAddress != DefaultManagementAddress {
		t.Fatalf("unexpected listeners: %+v", config)
	}
	if config.Database.URL != testDatabaseURL || config.Database.MaxConnections != DefaultDatabaseMaxConns || config.Database.MinConnections != DefaultDatabaseMinConns {
		t.Fatalf("unexpected database defaults: %+v", config.Database)
	}
	if config.MemoryLimitBytes != 0 || config.MaxProcs != 0 || !config.MetricsEnabled || config.Tracing.Enabled {
		t.Fatalf("unexpected runtime defaults: %+v", config)
	}

	environment := map[string]string{
		EnvAddress: ":9000", EnvManagementAddress: "127.0.0.1:9001", EnvServerURL: "https://api.example.com/",
		EnvCacheControl: "private", EnvAccessLog: "true", EnvMaxProcs: "3", EnvMemoryLimitMiB: "256",
		EnvLogLevel: "warn", EnvQueryTimeout: "2s", EnvShutdownTimeout: "3s", EnvDatabaseURL: testDatabaseURL,
		EnvDatabaseMaxConns: "7", EnvDatabaseMinConns: "1", EnvStatementCache: "32", EnvMetricsEnabled: "false",
		EnvTracingEnabled: "true", EnvOTLPEndpoint: "http://collector:4318", EnvTraceSampleRatio: "0.5",
	}
	result, err = Load([]string{"--address=:9100", "--max-procs=4", "--memory-limit-mib=128"}, mapLookup(environment), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	config = result.Config
	if config.PublicAddress != ":9100" || config.ManagementAddress != "127.0.0.1:9001" || config.ServerURL != "https://api.example.com" {
		t.Fatalf("CLI/ENV precedence failed: %+v", config)
	}
	if config.MaxProcs != 4 || config.MemoryLimitBytes != 128<<20 || config.LogLevel != slog.LevelWarn {
		t.Fatalf("runtime override failed: %+v", config)
	}
	if config.QueryTimeout != 2*time.Second || config.ShutdownTimeout != 3*time.Second || config.CacheControl != "private" || !config.AccessLog {
		t.Fatalf("server override failed: %+v", config)
	}
	if config.Database.MaxConnections != 7 || config.Database.MinConnections != 1 || config.Database.StatementCacheSize != 32 {
		t.Fatalf("database override failed: %+v", config.Database)
	}
	if config.MetricsEnabled || !config.Tracing.Enabled || config.Tracing.Endpoint != "http://collector:4318" || config.Tracing.SampleRatio != 0.5 {
		t.Fatalf("telemetry override failed: %+v", config)
	}
}

func TestLoadSplitDatabaseSettings(t *testing.T) {
	t.Parallel()
	lookup := mapLookup(map[string]string{
		EnvDatabaseHost: "db.example.com:5432", EnvDatabaseUsername: "read user", EnvDatabasePassword: "p@ss/word",
		EnvDatabaseName: "catalog", EnvDatabaseSSLMode: "require",
	})
	result, err := Load(nil, lookup, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Database.URL != "postgres://read%20user:p%40ss%2Fword@db.example.com:5432/catalog?sslmode=require" {
		t.Fatalf("unexpected URL: %s", result.Config.Database.URL)
	}
}

func TestLoadControlArgumentsDoNotReadEnvironment(t *testing.T) {
	t.Parallel()
	badEnvironment := mapLookup(map[string]string{EnvAccessLog: "not-a-boolean"})
	result, err := Load([]string{"--version"}, badEnvironment, &bytes.Buffer{})
	if err != nil || !result.ShowVersion {
		t.Fatalf("version result=%+v err=%v", result, err)
	}
	var output bytes.Buffer
	_, err = Load([]string{"--help"}, badEnvironment, &output)
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(output.String(), "-"+FlagDatabaseURL) {
		t.Fatalf("help err=%v output=%q", err, output.String())
	}
}

func TestLoadRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		environment map[string]string
		arguments   []string
		message     string
	}{
		{"boolean ENV", map[string]string{EnvAccessLog: "sometimes"}, nil, "must be a boolean"},
		{"integer ENV", map[string]string{EnvMaxProcs: "many"}, nil, "must be an integer"},
		{"memory ENV", map[string]string{EnvMemoryLimitMiB: "large"}, nil, "must be an integer"},
		{"duration ENV", map[string]string{EnvQueryTimeout: "soon"}, nil, "must be a duration"},
		{"shutdown duration ENV", map[string]string{EnvShutdownTimeout: "later"}, nil, "must be a duration"},
		{"max connections ENV", map[string]string{EnvDatabaseMaxConns: "many"}, nil, "must be an integer"},
		{"min connections ENV", map[string]string{EnvDatabaseMinConns: "few"}, nil, "must be an integer"},
		{"statement cache ENV", map[string]string{EnvStatementCache: "large"}, nil, "must be an integer"},
		{"metrics ENV", map[string]string{EnvMetricsEnabled: "enabled"}, nil, "must be a boolean"},
		{"tracing ENV", map[string]string{EnvTracingEnabled: "enabled"}, nil, "must be a boolean"},
		{"ratio ENV", map[string]string{EnvTraceSampleRatio: "half"}, nil, "must be a float"},
		{"unknown flag", validEnvironment(), []string{"--unknown"}, "flag provided but not defined"},
		{"positional", validEnvironment(), []string{"extra"}, "unexpected positional argument"},
		{"server URL", merge(validEnvironment(), EnvServerURL, "ftp://example.com"), nil, FlagServerURL},
		{"public address", merge(validEnvironment(), EnvAddress, "bad address"), nil, FlagAddress},
		{"management address", merge(validEnvironment(), EnvManagementAddress, "bad address"), nil, FlagManagementAddress},
		{"log level", merge(validEnvironment(), EnvLogLevel, "verbose"), nil, FlagLogLevel},
		{"database URL", map[string]string{EnvDatabaseURL: "http://example.com"}, nil, "absolute PostgreSQL URL"},
		{"missing database", map[string]string{}, nil, "split database settings"},
		{"split host", splitEnvironment("localhost", "discogs"), nil, "must include host and port"},
		{"split database", splitEnvironment("localhost:5432", "bad/name"), nil, "must be a database name"},
		{"max procs low", validEnvironment(), []string{"--max-procs=-1"}, FlagMaxProcs},
		{"memory low", validEnvironment(), []string{"--memory-limit-mib=-1"}, FlagMemoryLimitMiB},
		{"memory overflow", validEnvironment(), []string{"--memory-limit-mib=8796093022208"}, "too large"},
		{"max conns low", validEnvironment(), []string{"--db-max-conns=0"}, FlagDatabaseMaxConns},
		{"max conns high", validEnvironment(), []string{"--db-max-conns=1025"}, FlagDatabaseMaxConns},
		{"min conns low", validEnvironment(), []string{"--db-min-conns=-1"}, FlagDatabaseMinConns},
		{"min conns high", validEnvironment(), []string{"--db-max-conns=2", "--db-min-conns=3"}, FlagDatabaseMinConns},
		{"statement low", validEnvironment(), []string{"--db-statement-cache=-1"}, FlagStatementCache},
		{"statement high", validEnvironment(), []string{"--db-statement-cache=65537"}, FlagStatementCache},
		{"query timeout", validEnvironment(), []string{"--query-timeout=0s"}, "timeouts must be positive"},
		{"shutdown timeout", validEnvironment(), []string{"--shutdown-timeout=0s"}, "timeouts must be positive"},
		{"ratio low", validEnvironment(), []string{"--trace-sample-ratio=-0.1"}, FlagTraceSampleRatio},
		{"ratio high", validEnvironment(), []string{"--trace-sample-ratio=1.1"}, FlagTraceSampleRatio},
		{"missing endpoint", validEnvironment(), []string{"--tracing-enabled"}, FlagOTLPEndpoint},
		{"invalid endpoint", validEnvironment(), []string{"--tracing-enabled", "--otlp-endpoint=collector"}, FlagOTLPEndpoint},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(test.arguments, mapLookup(test.environment), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v, want substring %q", err, test.message)
			}
		})
	}
}

func TestInventoryIsDefensiveAndComplete(t *testing.T) {
	t.Parallel()
	first := Inventory()
	first[0].Flag = "changed"
	second := Inventory()
	if second[0].Flag != FlagAddress {
		t.Fatal("Inventory returned shared storage")
	}
	seenFlags := make(map[string]struct{}, len(second))
	seenEnvironment := make(map[string]struct{}, len(second))
	for _, definition := range second {
		if definition.Flag == "" || definition.Environment == "" || definition.Type == "" || definition.Requirement == "" || definition.Description == "" {
			t.Fatalf("incomplete definition: %+v", definition)
		}
		if _, exists := seenFlags[definition.Flag]; exists {
			t.Fatalf("duplicate flag: %s", definition.Flag)
		}
		if _, exists := seenEnvironment[definition.Environment]; exists {
			t.Fatalf("duplicate ENV: %s", definition.Environment)
		}
		seenFlags[definition.Flag] = struct{}{}
		seenEnvironment[definition.Environment] = struct{}{}
	}
	if description("missing") != "missing" {
		t.Fatal("missing description fallback changed")
	}
}

func TestREADMEContainsImplementedConfigurationInventory(t *testing.T) {
	t.Parallel()
	document, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(document)
	for _, definition := range Inventory() {
		flagText := "`--" + definition.Flag + "`"
		environmentText := "`" + definition.Environment + "`"
		if !strings.Contains(content, flagText) || !strings.Contains(content, environmentText) {
			t.Fatalf("README inventory is missing %s / %s", flagText, environmentText)
		}
	}
}

func validEnvironment() map[string]string {
	return map[string]string{EnvDatabaseURL: testDatabaseURL}
}

func splitEnvironment(host, database string) map[string]string {
	return map[string]string{
		EnvDatabaseHost: host, EnvDatabaseUsername: "reader", EnvDatabasePassword: "secret", EnvDatabaseName: database,
	}
}

func merge(source map[string]string, key, value string) map[string]string {
	result := make(map[string]string, len(source)+1)
	for sourceKey, sourceValue := range source {
		result[sourceKey] = sourceValue
	}
	result[key] = value
	return result
}

func mapLookup(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
