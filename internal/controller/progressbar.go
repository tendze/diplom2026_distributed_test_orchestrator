package controller

import (
	progress "github.com/cheggaaa/pb/v3"
)

type ProgressReporter struct {
	bar *progress.ProgressBar
}

func NewProgressReporter(total int64) *ProgressReporter {
	bar := progress.Start64(total)

	return &ProgressReporter{bar: bar}
}

func (p *ProgressReporter) Add(n int64) {
	p.bar.Add64(n)
}

func (p *ProgressReporter) Finish() {
	p.bar.Finish()
}
