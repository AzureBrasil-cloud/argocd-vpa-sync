package domain

import (
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

// RequestHeadroom is the headroom last applied over the VPA recommendation
// for one container: the request written was recommendation / (1 - p/100),
// so the recommendation (the pod's actual usage) sits at (100-p)% of the
// request -- leaving room before a utilization-based HPA scales out. It is
// recorded only once a write-back succeeds, and from then on eligibility
// compares the live request against the recommendation plus this headroom,
// so a container already carrying it isn't offered a change "down" to the
// bare recommendation. Nil means no headroom (0%) for that resource.
type RequestHeadroom struct {
	CPUPercent    *float64  `json:"cpuPercent,omitempty"`
	MemoryPercent *float64  `json:"memoryPercent,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// RequestHeadroomKey is the StateDocument.RequestHeadrooms key for one
// container.
func RequestHeadroomKey(vpaNamespace, vpaName, containerName string) string {
	return vpaNamespace + "/" + vpaName + "/" + containerName
}

// RequestHeadroomFor returns the headroom recorded for one container, or the
// zero value (no headroom) when none is.
func (d *StateDocument) RequestHeadroomFor(vpaNamespace, vpaName, containerName string) RequestHeadroom {
	if d == nil {
		return RequestHeadroom{}
	}
	return d.RequestHeadrooms[RequestHeadroomKey(vpaNamespace, vpaName, containerName)]
}

// NonZeroPercent treats a 0% headroom the same as none (nil), so it neither
// overrides the request, changes an idempotency key, nor gets recorded.
func NonZeroPercent(p *float64) *float64 {
	if p == nil || *p == 0 {
		return nil
	}
	v := *p
	return &v
}

// RequestWithHeadroom returns recommended scaled up by headroomPct (see
// ScaleForHeadroom). A nil recommended stays nil, and a nil or zero
// headroom returns recommended unchanged.
func RequestWithHeadroom(recommended *resource.Quantity, headroomPct *float64, isCPU bool) (*resource.Quantity, error) {
	if recommended == nil || headroomPct == nil || *headroomPct == 0 {
		return recommended, nil
	}
	q, err := ScaleForHeadroom(*recommended, *headroomPct, isCPU)
	if err != nil {
		return nil, err
	}
	return &q, nil
}
