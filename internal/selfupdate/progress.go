// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import "io"

type Stage string

const (
	StageCheckRelease  Stage = "check_release"
	StageVerifyRelease Stage = "verify_release"
	StagePrepare       Stage = "prepare"
	StageDownload      Stage = "download"
	StageVerifyBinary  Stage = "verify_binary"
	StageInstall       Stage = "install"
)

// Progress describes work in progress, not a successful installation. Update's
// return value remains authoritative for success or failure.
type Progress struct {
	Stage   Stage
	Version string
	// Downloaded and Total count binary bytes; Total <= 0 means unknown.
	Downloaded int64
	Total      int64
	// Complete marks a fully downloaded body, before checksum/identity checks.
	Complete bool
}

// ProgressFunc is called synchronously during Update. It may be nil.
type ProgressFunc func(Progress)

func (report ProgressFunc) emit(progress Progress) {
	if report != nil {
		report(progress)
	}
}

type downloadProgressWriter struct {
	writer   io.Writer
	report   ProgressFunc
	progress Progress
}

func (writer *downloadProgressWriter) Write(data []byte) (int, error) {
	n, err := writer.writer.Write(data)
	writer.progress.Downloaded += int64(n)
	writer.report.emit(writer.progress)
	return n, err
}
