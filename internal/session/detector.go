package session

import (
	"log/slog"

	"github.com/danni2019/starSling/internal/config"
)

type Status struct {
	InSession bool
	Reason    string
}

type Detector struct {
	liveMD config.LiveMDConfig
	logger *slog.Logger
}

func NewDetector(liveMD config.LiveMDConfig, logger *slog.Logger) *Detector {
	return &Detector{liveMD: liveMD, logger: logger}
}

func (d *Detector) Check() Status {
	if d.liveMD.Host == "" || d.liveMD.Port == 0 {
		return Status{InSession: false, Reason: "live-md not configured"}
	}

	d.logger.Debug("session check stub", "host", d.liveMD.Host, "port", d.liveMD.Port)
	return Status{InSession: false, Reason: "session detection not implemented"}
}
