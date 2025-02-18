package gohlslib

import (
	"context"
	"fmt"
	"time"
)

type clientTrack struct {
	track            *Track
	onData           func(pts int64, dts int64, data [][]byte)
	lastAbsoluteTime *time.Time
	startRTC         time.Time
}

func (t *clientTrack) absoluteTime() (time.Time, bool) {
	if t.lastAbsoluteTime == nil {
		return zero, false
	}
	return *t.lastAbsoluteTime, true
}

func (t *clientTrack) handleData(
	ctx context.Context,
	pts int64,
	dts int64,
	ntp *time.Time,
	data [][]byte,
) error {
	// silently discard packets prior to the first packet of the leading track
	if pts < 0 {
		return nil
	}

	// synchronize time
	elapsed := time.Since(t.startRTC)
	dtsDuration := timestampToDuration(dts, t.track.ClockRate)
	if dtsDuration > elapsed {
		diff := dtsDuration - elapsed
		if diff > clientMaxDTSRTCDiff {
			return fmt.Errorf("difference between DTS and RTC is too big")
		}

		select {
		case <-time.After(diff):
		case <-ctx.Done():
			return fmt.Errorf("terminated")
		}
	}

	t.lastAbsoluteTime = ntp
	t.onData(pts, dts, data)
	return nil
}
