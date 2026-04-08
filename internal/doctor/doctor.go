package doctor

import (
	"fmt"
	"io"
	"strings"

	"github.com/danni2019/starSling/internal/config"
	"github.com/danni2019/starSling/internal/configstore"
	"github.com/danni2019/starSling/internal/live"
	"github.com/danni2019/starSling/internal/metadata"
	internalruntime "github.com/danni2019/starSling/internal/runtime"
	"github.com/danni2019/starSling/internal/settingsstore"
)

type Severity string

const (
	SeverityOK   Severity = "ok"
	SeverityWarn Severity = "warn"
	SeverityFail Severity = "fail"
)

type Check struct {
	Name     string
	Severity Severity
	Detail   string
}

type Report struct {
	Checks []Check
}

var (
	runtimePlatformFn   = live.RuntimePlatform
	bootstrapScriptFn   = internalruntime.BootstrapScriptPath
	bundledPythonPathFn = live.BundledPythonPath
	defaultConfigFn     = config.Default
	configDirFn         = configstore.Dir
	metadataDirFn       = settingsstore.Dir
	metadataSourcesFn   = metadata.SourcesFilePath
)

func Collect() Report {
	report := Report{}
	report.Checks = append(report.Checks, checkRuntimePlatform())
	report.Checks = append(report.Checks, checkBootstrapScript())
	report.Checks = append(report.Checks, checkMetadataSourcesConfig())
	report.Checks = append(report.Checks, checkBundledPython())
	report.Checks = append(report.Checks, checkEmbeddedDefaults())
	report.Checks = append(report.Checks, checkConfigDir())
	report.Checks = append(report.Checks, checkMetadataDir())
	return report
}

func (r Report) ExitCode() int {
	if r.HasFailures() {
		return 1
	}
	return 0
}

func (r Report) HasFailures() bool {
	return r.Count(SeverityFail) > 0
}

func (r Report) Count(severity Severity) int {
	total := 0
	for _, check := range r.Checks {
		if check.Severity == severity {
			total++
		}
	}
	return total
}

func (r Report) WriteTo(w io.Writer) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, "starSling doctor")
	for _, check := range r.Checks {
		fmt.Fprintf(w, "[%s] %s: %s\n", check.Severity, check.Name, check.Detail)
	}
	fmt.Fprintf(
		w,
		"\nSummary: %d ok, %d warn, %d fail\n",
		r.Count(SeverityOK),
		r.Count(SeverityWarn),
		r.Count(SeverityFail),
	)
}

func checkRuntimePlatform() Check {
	platform := strings.TrimSpace(runtimePlatformFn())
	if platform == "" {
		return Check{
			Name:     "runtime platform",
			Severity: SeverityFail,
			Detail:   "current GOOS/GOARCH is not supported by the runtime resolver",
		}
	}
	return Check{
		Name:     "runtime platform",
		Severity: SeverityOK,
		Detail:   platform,
	}
}

func checkBootstrapScript() Check {
	path, err := bootstrapScriptFn()
	if err != nil {
		return Check{
			Name:     "bootstrap script",
			Severity: SeverityFail,
			Detail:   err.Error(),
		}
	}
	return Check{
		Name:     "bootstrap script",
		Severity: SeverityOK,
		Detail:   path,
	}
}

func checkMetadataSourcesConfig() Check {
	path, err := metadataSourcesFn()
	if err != nil {
		return Check{
			Name:     "metadata sources config",
			Severity: SeverityFail,
			Detail:   err.Error(),
		}
	}
	return Check{
		Name:     "metadata sources config",
		Severity: SeverityOK,
		Detail:   path,
	}
}

func checkBundledPython() Check {
	path := strings.TrimSpace(bundledPythonPathFn())
	if path == "" {
		return Check{
			Name:     "bundled python",
			Severity: SeverityWarn,
			Detail:   "not found; run scripts/bootstrap_python.sh or use 'Setup Python runtime' before entering Live",
		}
	}
	return Check{
		Name:     "bundled python",
		Severity: SeverityOK,
		Detail:   path,
	}
}

func checkEmbeddedDefaults() Check {
	cfg, err := defaultConfigFn()
	if err != nil {
		return Check{
			Name:     "embedded default config",
			Severity: SeverityFail,
			Detail:   err.Error(),
		}
	}

	if strings.TrimSpace(cfg.LiveMD.Username) != "" || strings.TrimSpace(cfg.LiveMD.Password) != "" {
		return Check{
			Name:     "embedded default config",
			Severity: SeverityFail,
			Detail:   "embedded defaults must not include live credentials",
		}
	}

	host := strings.TrimSpace(cfg.LiveMD.Host)
	if host != "" {
		return Check{
			Name:     "embedded default config",
			Severity: SeverityFail,
			Detail:   fmt.Sprintf("embedded default host must remain unset (got %q)", host),
		}
	}
	if cfg.LiveMD.Port != 0 {
		return Check{
			Name:     "embedded default config",
			Severity: SeverityFail,
			Detail:   fmt.Sprintf("embedded default port must remain unset (got %d)", cfg.LiveMD.Port),
		}
	}

	return Check{
		Name:     "embedded default config",
		Severity: SeverityOK,
		Detail:   "host is unset and port is 0; configure real values in Config before entering Live",
	}
}

func checkConfigDir() Check {
	path, err := configDirFn()
	if err != nil {
		return Check{
			Name:     "config directory",
			Severity: SeverityFail,
			Detail:   err.Error(),
		}
	}
	return Check{
		Name:     "config directory",
		Severity: SeverityOK,
		Detail:   path,
	}
}

func checkMetadataDir() Check {
	path, err := metadataDirFn()
	if err != nil {
		return Check{
			Name:     "metadata directory",
			Severity: SeverityFail,
			Detail:   err.Error(),
		}
	}
	return Check{
		Name:     "metadata directory",
		Severity: SeverityOK,
		Detail:   path,
	}
}
