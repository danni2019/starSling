package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/danni2019/starSling/internal/doctor"
)

func TestRunDoctorCommandOutputsReport(t *testing.T) {
	orig := doctorCollectFn
	defer func() { doctorCollectFn = orig }()

	doctorCollectFn = func() doctor.Report {
		return doctor.Report{
			Checks: []doctor.Check{
				{Name: "runtime platform", Severity: doctor.SeverityOK, Detail: "macos-arm64"},
			},
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "runtime platform") {
		t.Fatalf("expected doctor output, got %q", stdout.String())
	}
}

func TestRunDoctorCommandReturnsNonZeroOnFailures(t *testing.T) {
	orig := doctorCollectFn
	defer func() { doctorCollectFn = orig }()

	doctorCollectFn = func() doctor.Report {
		return doctor.Report{
			Checks: []doctor.Check{
				{Name: "metadata sources config", Severity: doctor.SeverityFail, Detail: "missing"},
			},
		}
	}

	code := run([]string{"doctor"}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestRunDoctorCommandRejectsExtraArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"doctor", "extra"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage exit code, got %d", code)
	}
	if !strings.Contains(stderr.String(), "usage: starsling doctor") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"unknown"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "usage: starsling [doctor]") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}
