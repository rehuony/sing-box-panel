// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"encoding/json"
	"errors"

	"github.com/rehuony/sing-box-panel/internal/application"
)

// Keep the complete UTF-8 SSE frame within the browser decoder's byte limit.
const dashboardFrameLimit = 1 << 20
const dashboardFrameOverhead = len("event: dashboard\ndata: \n\n")

func encodeDashboardSnapshot(snapshot application.DashboardStreamSnapshot) ([]byte, error) {
	data, err := json.Marshal(snapshot)
	if err != nil || len(data)+dashboardFrameOverhead <= dashboardFrameLimit {
		return data, err
	}

	// Keep the largest newest-first prefix that fits. The cursor identifies the
	// last retained transition, so the omitted prefix stays unknown in the UI.
	items := snapshot.Runtime24H.Items
	var bounded []byte
	low, high := 1, len(items)-1
	for low <= high {
		count := low + (high-low)/2
		snapshot.Runtime24H.Items = items[:count]
		last := items[count-1]
		snapshot.Runtime24H.Next = &application.RuntimeTransitionCursor{OccurredAt: last.OccurredAt, ID: last.ID}
		data, err = json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		if len(data)+dashboardFrameOverhead <= dashboardFrameLimit {
			bounded = data
			low = count + 1
		} else {
			high = count - 1
		}
	}
	if bounded == nil {
		return nil, errors.New("dashboard snapshot exceeds the frame limit")
	}
	return bounded, nil
}
