package clashapi

import (
	"sync/atomic"
)

type TrafficTracker struct {
	upload   atomic.Int64
	download atomic.Int64
	lastUp   atomic.Int64
	lastDn   atomic.Int64
}

func NewTrafficTracker() *TrafficTracker { return &TrafficTracker{} }

func (t *TrafficTracker) AddUpload(n int64)   { t.upload.Add(n) }
func (t *TrafficTracker) AddDownload(n int64) { t.download.Add(n) }

func (t *TrafficTracker) Totals() (int64, int64) {
	return t.upload.Load(), t.download.Load()
}

func (t *TrafficTracker) Snapshot() (int64, int64) {
	up := t.upload.Load()
	dn := t.download.Load()
	prevUp := t.lastUp.Swap(up)
	prevDn := t.lastDn.Swap(dn)
	return up - prevUp, dn - prevDn
}