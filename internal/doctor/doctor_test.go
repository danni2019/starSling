package doctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/danni2019/starSling/internal/config"
)

func TestCollectReturnsHealthyReportWhenReleaseContractIsIntact(t *testing.T) {
	restore := stubDoctorDeps(t)
	defer restore()

	runtimePlatformFn = func() string { return "macos-arm64" }
	bootstrapScriptFn = func() (string, error) { return "/tmp/scripts/bootstrap_python.sh", nil }
	metadataSourcesFn = func() (string, error) { return "/tmp/config/metadata.sources.json", nil }
	bundledPythonPathFn = func() string { return "/tmp/runtime/macos-arm64/venv/bin/python" }
	configDirFn = func() (string, error) { return "/tmp/starsling/configs", nil }
	metadataDirFn = func() (string, error) { return "/tmp/starsling/metadata", nil }
	defaultConfigFn = func() (config.Config, error) {
		return config.Config{
			Runtime: config.RuntimeConfig{CheckIntervalSeconds: 5, IdleLogIntervalSeconds: 30},
			LiveMD: config.LiveMDConfig{
				API:      "ctp",
				Protocol: "tcp",
				Host:     "",
				Port:     0,
			},
		}, nil
	}

	report := Collect()
	if report.HasFailures() {
		t.Fatalf("expected no failures, got %+v", report.Checks)
	}
	if report.Count(SeverityWarn) != 0 {
		t.Fatalf("expected no warnings, got %+v", report.Checks)
	}
	if report.ExitCode() != 0 {
		t.Fatalf("expected zero exit code, got %d", report.ExitCode())
	}
}

func TestCollectWarnsWhenBundledPythonIsMissing(t *testing.T) {
	restore := stubDoctorDeps(t)
	defer restore()

	runtimePlatformFn = func() string { return "macos-arm64" }
	bootstrapScriptFn = func() (string, error) { return "/tmp/scripts/bootstrap_python.sh", nil }
	metadataSourcesFn = func() (string, error) { return "/tmp/config/metadata.sources.json", nil }
	bundledPythonPathFn = func() string { return "" }
	configDirFn = func() (string, error) { return "/tmp/starsling/configs", nil }
	metadataDirFn = func() (string, error) { return "/tmp/starsling/metadata", nil }
	defaultConfigFn = func() (config.Config, error) {
		return config.Config{
			Runtime: config.RuntimeConfig{CheckIntervalSeconds: 5, IdleLogIntervalSeconds: 30},
			LiveMD: config.LiveMDConfig{
				API:      "ctp",
				Protocol: "tcp",
				Host:     "",
				Port:     0,
			},
		}, nil
	}

	report := Collect()
	if report.HasFailures() {
		t.Fatalf("expected warnings only, got %+v", report.Checks)
	}
	if !hasCheck(report, "bundled python", SeverityWarn) {
		t.Fatalf("expected bundled python warning, got %+v", report.Checks)
	}
	if report.ExitCode() != 0 {
		t.Fatalf("expected zero exit code with warnings only, got %d", report.ExitCode())
	}
}

func TestCollectFailsWhenEmbeddedDefaultsLeakLiveEndpoint(t *testing.T) {
	restore := stubDoctorDeps(t)
	defer restore()

	runtimePlatformFn = func() string { return "macos-arm64" }
	bootstrapScriptFn = func() (string, error) { return "/tmp/scripts/bootstrap_python.sh", nil }
	metadataSourcesFn = func() (string, error) { return "/tmp/config/metadata.sources.json", nil }
	bundledPythonPathFn = func() string { return "/tmp/runtime/macos-arm64/venv/bin/python" }
	configDirFn = func() (string, error) { return "/tmp/starsling/configs", nil }
	metadataDirFn = func() (string, error) { return "/tmp/starsling/metadata", nil }
	defaultConfigFn = func() (config.Config, error) {
		return config.Config{
			Runtime: config.RuntimeConfig{CheckIntervalSeconds: 5, IdleLogIntervalSeconds: 30},
			LiveMD: config.LiveMDConfig{
				API:      "ctp",
				Protocol: "tcp",
				Host:     "test-md-front.invalid",
				Port:     41213,
			},
		}, nil
	}

	report := Collect()
	if !report.HasFailures() {
		t.Fatalf("expected failure, got %+v", report.Checks)
	}
	if !hasCheck(report, "embedded default config", SeverityFail) {
		t.Fatalf("expected embedded default config failure, got %+v", report.Checks)
	}
	if report.ExitCode() != 1 {
		t.Fatalf("expected non-zero exit code, got %d", report.ExitCode())
	}
}

func TestCollectFailsWhenMetadataSourcesConfigIsMissing(t *testing.T) {
	restore := stubDoctorDeps(t)
	defer restore()

	runtimePlatformFn = func() string { return "macos-arm64" }
	bootstrapScriptFn = func() (string, error) { return "/tmp/scripts/bootstrap_python.sh", nil }
	metadataSourcesFn = func() (string, error) {
		return "", errors.New("metadata sources config not found (config/metadata.sources.json)")
	}
	bundledPythonPathFn = func() string { return "/tmp/runtime/macos-arm64/venv/bin/python" }
	configDirFn = func() (string, error) { return "/tmp/starsling/configs", nil }
	metadataDirFn = func() (string, error) { return "/tmp/starsling/metadata", nil }
	defaultConfigFn = func() (config.Config, error) {
		return config.Config{
			Runtime: config.RuntimeConfig{CheckIntervalSeconds: 5, IdleLogIntervalSeconds: 30},
			LiveMD: config.LiveMDConfig{
				API:      "ctp",
				Protocol: "tcp",
				Host:     "",
				Port:     0,
			},
		}, nil
	}

	report := Collect()
	if !hasCheck(report, "metadata sources config", SeverityFail) {
		t.Fatalf("expected metadata sources config failure, got %+v", report.Checks)
	}
}

func TestWriteToIncludesSummary(t *testing.T) {
	report := Report{
		Checks: []Check{
			{Name: "runtime platform", Severity: SeverityOK, Detail: "macos-arm64"},
			{Name: "bundled python", Severity: SeverityWarn, Detail: "not found"},
		},
	}

	var sb strings.Builder
	report.WriteTo(&sb)
	out := sb.String()
	if !strings.Contains(out, "starSling doctor") {
		t.Fatalf("expected header in output: %q", out)
	}
	if !strings.Contains(out, "Summary: 1 ok, 1 warn, 0 fail") {
		t.Fatalf("expected summary in output: %q", out)
	}
}

func stubDoctorDeps(t *testing.T) func() {
	t.Helper()
	origRuntimePlatformFn := runtimePlatformFn
	origBootstrapScriptFn := bootstrapScriptFn
	origMetadataSourcesFn := metadataSourcesFn
	origBundledPythonPathFn := bundledPythonPathFn
	origConfigDirFn := configDirFn
	origMetadataDirFn := metadataDirFn
	origDefaultConfigFn := defaultConfigFn

	return func() {
		runtimePlatformFn = origRuntimePlatformFn
		bootstrapScriptFn = origBootstrapScriptFn
		metadataSourcesFn = origMetadataSourcesFn
		bundledPythonPathFn = origBundledPythonPathFn
		configDirFn = origConfigDirFn
		metadataDirFn = origMetadataDirFn
		defaultConfigFn = origDefaultConfigFn
	}
}

func hasCheck(report Report, name string, severity Severity) bool {
	for _, check := range report.Checks {
		if check.Name == name && check.Severity == severity {
			return true
		}
	}
	return false
}
